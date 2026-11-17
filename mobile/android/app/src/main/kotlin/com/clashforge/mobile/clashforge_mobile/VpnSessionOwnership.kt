package com.clashforge.mobile.clashforge_mobile

/** Process-wide identity gate. Never run creation, logging or disposal under it. */
internal class VpnSessionOwnership<T : Any, P : Any>(
    private val closeTun: (T) -> Unit,
    private val stopProcess: (P) -> Unit,
) {
    internal class Session<T, P>(val owner: Any) {
        internal var tun: T? = null
        internal var process: P? = null
        internal var cancelled = false
        internal var inFlight = 0
        internal var cleaning = false
        internal var queued = false
        internal var schedule: ((() -> Unit) -> Unit)? = null
        internal var failure: Throwable? = null
        internal var retired: (() -> Unit)? = null
    }
    private val gate = Object()
    private var current: Session<T, P>? = null

    fun start(owner: Any, retirePrevious: Boolean = true, publish: (Session<T, P>) -> Unit): Session<T, P>? {
        val old = synchronized(gate) {
            if (current?.owner === owner && current?.cancelled == false) return null
            current
        }
        old?.let {
            check(retirePrevious && retire(it) {}) { "Previous VPN cleanup pending" }
        }
        return synchronized(gate) {
            check(current == null) { "Previous VPN still owns resources" }
            Session<T, P>(owner).also { current = it; publish(it) }
        }
    }
    fun isCurrent(session: Session<T, P>): Boolean = synchronized(gate) {
        current === session && !session.cancelled
    }
    fun <R> withCurrent(session: Session<T, P>, action: () -> R): R? = synchronized(gate) {
        if (current === session && !session.cancelled) action() else null
    }
    private fun begin(session: Session<T, P>): Boolean = synchronized(gate) {
        if (current !== session || session.cancelled) false
        else { session.inFlight++; true }
    }
    private fun end(session: Session<T, P>) {
        val cancelled = synchronized(gate) {
            session.inFlight--; gate.notifyAll(); session.cancelled
        }
        if (cancelled) try {
            val schedule = synchronized(gate) { session.schedule }
            if (schedule != null) requestRetire(session, {}, schedule) else drain(session)
        } catch (_: Exception) {
            // Failure/handles remain owned. STOP await/retry observes the same failure.
        }
    }
    /** A cancellable publication lease: slow Binder/log callbacks do not hold the gate. */
    fun duringSession(session: Session<T, P>, action: () -> Unit): Boolean {
        if (!begin(session)) return false
        try { action(); return true } finally { end(session) }
    }
    fun acquireTun(session: Session<T, P>, create: () -> T?): T? {
        if (!begin(session)) return null
        try {
            val tun = create() ?: return null
            synchronized(gate) { session.tun = tun }
            return if (isCurrent(session)) tun else null
        } finally { end(session) }
    }
    fun acquireProcess(session: Session<T, P>, installed: (P) -> Unit = {}, create: (T) -> P?): P? {
        if (!begin(session)) return null
        try {
            val tun = synchronized(gate) { session.tun } ?: return null
            val process = create(tun) ?: return null
            synchronized(gate) { session.process = process }
            if (!isCurrent(session)) return null
            installed(process) // may log/block; STOP can still cancel immediately
            return if (isCurrent(session)) process else null
        } finally { end(session) }
    }
    /** Adopt immediately after OS spawn, before fallible stdin restoration. */
    fun recordProcess(session: Session<T, P>, process: P) = synchronized(gate) {
        check(current === session && session.inFlight > 0)
        session.process = process
    }
    /** Cancellation only: safe on Android's main thread, even during native I/O. */
    fun cancel(session: Session<T, P>) = synchronized(gate) {
        if (current === session) { session.cancelled = true; gate.notifyAll() }
    }
    /** Returns false (pending) immediately if creation/disposal is in flight. */
    fun retire(session: Session<T, P>, publish: () -> Unit): Boolean {
        synchronized(gate) {
            if (current !== session) return true
            session.cancelled = true
            if (session.retired == null) session.retired = publish
        }
        return drain(session)
    }
    /** Schedule physical cleanup independently of the bounded control caller.
     * The scheduler must enqueue, never execute inline. One queued/running job per lease.
     */
    fun requestRetire(session: Session<T, P>, publish: () -> Unit, schedule: (() -> Unit) -> Unit) {
        synchronized(gate) {
            if (current !== session) return
            session.cancelled = true
            if (session.retired == null) session.retired = publish
            session.schedule = schedule
            if (session.queued || session.cleaning || session.inFlight > 0) return
            session.queued = true
            session.failure = null // a queued retry must not expose the previous failure
        }
        try {
            schedule {
                try { drain(session, scheduled = true) } catch (_: Exception) {
                    // awaitRetired observes the recorded failure; ownership is retained.
                }
            }
        } catch (e: Exception) {
            synchronized(gate) {
                session.queued = false
                session.failure = e
                gate.notifyAll()
            }
            throw e
        }
    }
    private fun drain(session: Session<T, P>, scheduled: Boolean = false): Boolean {
        synchronized(gate) {
            if (current !== session) return true
            if (session.queued && !scheduled) return false
            if (scheduled) session.queued = false
            if (session.inFlight > 0 || session.cleaning || session.retired == null) return false
            session.cleaning = true
            session.failure = null
        }
        var failure: Throwable? = null
        val process = synchronized(gate) { session.process }
        val tun = synchronized(gate) { session.tun }
        try {
            process?.let {
                try { stopProcess(it); synchronized(gate) { session.process = null } }
                catch (e: Exception) { failure = e }
            }
            tun?.let {
                try { closeTun(it); synchronized(gate) { session.tun = null } }
                catch (e: Exception) { if (failure == null) failure = e else failure!!.addSuppressed(e) }
            }
            if (failure == null) {
                // Lifecycle/Binder callbacks can also stall. Keep the cancelled
                // lease non-replaceable, but never hold the gate while publishing.
                try { session.retired?.invoke() } catch (e: Exception) { failure = e }
            }
        } finally {
            synchronized(gate) {
                session.cleaning = false
                session.failure = failure
                if (failure == null && session.process == null && session.tun == null) {
                    current = null
                    session.retired = null
                }
                gate.notifyAll()
            }
        }
        failure?.let { throw it }
        return true
    }
    /** Worker-only bounded wait; never called from Android lifecycle callbacks. */
    fun awaitRetired(session: Session<T, P>, timeoutMs: Long): Boolean {
        val deadline = System.nanoTime() + timeoutMs * 1_000_000
        synchronized(gate) {
            while (current === session) {
                session.failure?.let { throw it }
                val remaining = deadline - System.nanoTime()
                if (remaining <= 0) return false
                gate.wait((remaining / 1_000_000).coerceAtLeast(1))
            }
            return true
        }
    }
    fun currentSession(): Session<T, P>? = synchronized(gate) { current }
}
