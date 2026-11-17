package com.clashforge.mobile.clashforge_mobile

import java.util.concurrent.atomic.AtomicLong

/** Shared service/MethodChannel completion ledger, independent of Android for JVM tests. */
internal class VpnControlRequests {
    private data class Pending(val reply: (String) -> Unit, var cancelTimer: (() -> Unit)? = null)
    private val ids = AtomicLong()
    private val pending = mutableMapOf<Long, Pending>()

    fun submit(
        scheduleTimeout: ((() -> Unit) -> (() -> Unit)),
        cancel: (Long) -> Unit,
        dispatch: (Long) -> Unit,
        reply: (String) -> Unit,
    ): Long {
        val id = ids.incrementAndGet()
        synchronized(this) { pending[id] = Pending(reply) }
        val timer = scheduleTimeout {
            if (complete(id, "error:VPN operation timed out")) cancel(id)
        }
        synchronized(this) { pending[id]?.cancelTimer = timer }
        if (!isPending(id)) timer()
        try { if (isPending(id)) dispatch(id) } catch (e: Exception) { fail(id, e) }
        return id
    }
    fun isPending(id: Long): Boolean = synchronized(this) { pending.containsKey(id) }
    fun complete(id: Long?, value: String): Boolean {
        if (id == null) return true // notification/tile/permission callback
        val request = synchronized(this) { pending.remove(id) } ?: return false
        request.cancelTimer?.invoke()
        request.reply(value)
        return true
    }
    fun fail(id: Long?, failure: Throwable): Boolean =
        complete(id, "error:${failure.message ?: failure.javaClass.simpleName}")

    /** Actual STOP controller: acknowledge only after disposal and lifecycle retirement. */
    fun stop(id: Long?, cleanup: () -> Unit) {
        try { cleanup(); complete(id, "stopped") }
        catch (e: Exception) { fail(id, e) }
    }
    /** Actual START commit point: a failed/cancelled/late start never acknowledges success. */
    fun started(id: Long?, ready: () -> Unit): Boolean = try {
        ready(); complete(id, "started")
    } catch (e: Exception) { fail(id, e); false }
}
