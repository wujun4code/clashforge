package com.clashforge.mobile.clashforge_mobile

import org.junit.Assert.*
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

class VpnOwnershipRoutingTest {
    private class Resource { var closes = 0 }
    private class Fixture {
        val gate = VpnSessionOwnership<Resource, Resource>({ it.closes++ }, { it.closes++ })
        var running = false
        var pid = -1
        var foregroundStops = 0
        var selfStops = 0
        fun start(owner: Any) = gate.start(owner) { running = true }!!
        fun attach(s: VpnSessionOwnership.Session<Resource, Resource>): Pair<Resource, Resource> {
            val tun = gate.acquireTun(s) { Resource() }!!
            val process = gate.acquireProcess(s) { Resource() }!!
            gate.withCurrent(s) { pid = 42 }
            return tun to process
        }
        fun stop(s: VpnSessionOwnership.Session<Resource, Resource>) = gate.retire(s) {
            running = false; pid = -1; foregroundStops++; selfStops++
        }
    }

    @Test fun oldFinallyCannotRetireNewSessionOnSameInstance() = lateFinally(false)
    @Test fun oldFinallyCannotRetireRecreatedServiceSession() = lateFinally(true)

    private fun lateFinally(recreated: Boolean) {
        val f = Fixture()
        val owner = Any()
        val a = f.start(owner)
        val old = f.attach(a)
        val waiting = CountDownLatch(1)
        val release = CountDownLatch(1)
        val failure = AtomicReference<Throwable?>()
        val worker = Thread {
            try {
                waiting.countDown()
                check(release.await(3, TimeUnit.SECONDS))
                f.stop(a) // production's process-exit finally callback
            } catch (t: Throwable) { failure.set(t) }
        }
        worker.start()
        try {
            assertTrue(waiting.await(3, TimeUnit.SECONDS))
            f.stop(a)
            val b = f.start(if (recreated) Any() else owner)
            val fresh = f.attach(b)
            release.countDown()
            worker.join(3000)
            assertFalse(worker.isAlive)
            failure.get()?.let { throw it }
            assertTrue("B must remain owner", f.gate.isCurrent(b))
            assertTrue(f.running)
            assertEquals(42, f.pid)
            assertEquals(1, f.foregroundStops)
            assertEquals(1, f.selfStops)
            assertEquals(1, old.first.closes)
            assertEquals(1, old.second.closes)
            assertEquals(0, fresh.first.closes)
            assertEquals(0, fresh.second.closes)
            f.stop(b)
        } finally { release.countDown(); worker.join(3000) }
    }

    @Test fun replacementCleansOldResourcesWithoutOldLifecyclePublication() {
        val f = Fixture()
        val a = f.start(Any())
        val old = f.attach(a)
        val b = f.start(Any())
        f.stop(a) // late onDestroy/onRevoke from old Service
        assertTrue(f.gate.isCurrent(b))
        assertEquals(0, f.selfStops)
        assertEquals(1, old.first.closes)
        assertEquals(1, old.second.closes)
        f.stop(b)
    }

    @Test fun cancelledStartupCannotAcquireResourcesOrPublish() {
        val f = Fixture()
        val a = f.start(Any())
        f.stop(a)
        var creates = 0
        assertNull(f.gate.acquireTun(a) { creates++; Resource() })
        assertNull(f.gate.acquireProcess(a) { creates++; Resource() })
        f.gate.withCurrent(a) { creates++ }
        assertEquals(0, creates)
        f.stop(a)
        assertEquals(1, f.selfStops)
    }

    @Test fun duplicateStartIsIgnoredAndProcessFailureStillClosesTun() {
        val f = Fixture()
        val owner = Any()
        val a = f.start(owner)
        assertNull(f.gate.start(owner) { fail("duplicate publication") })
        val tun = f.gate.acquireTun(a) { Resource() }!!
        try {
            f.gate.acquireProcess(a) { throw IllegalStateException("spawn failed") }
            fail("must propagate")
        } catch (_: IllegalStateException) { f.stop(a) }
        assertEquals(1, tun.closes)
        assertFalse(f.running)
    }

    @Test fun resourceInstallationAndRetirementAreSerialized() {
        val f = Fixture()
        val a = f.start(Any())
        val tun = f.gate.acquireTun(a) { Resource() }!!
        val process = Resource()
        val creating = CountDownLatch(1)
        val release = CountDownLatch(1)
        val stopping = CountDownLatch(1)
        val events = java.util.Collections.synchronizedList(mutableListOf<String>())
        val failure = AtomicReference<Throwable?>()
        val installer = Thread {
            try {
                f.gate.acquireProcess(a, installed = { events += "installed" }) {
                    creating.countDown()
                    check(release.await(3, TimeUnit.SECONDS))
                    process
                }
            } catch (t: Throwable) { failure.set(t) }
        }
        val stopper = Thread {
            try {
                stopping.countDown()
                f.gate.retire(a) { events += "retired" }
            } catch (t: Throwable) { failure.set(t) }
        }
        installer.start()
        try {
            assertTrue(creating.await(3, TimeUnit.SECONDS))
            stopper.start()
            assertTrue(stopping.await(3, TimeUnit.SECONDS))
            stopper.join(300)
            assertFalse("STOP must return pending before slow spawn finishes", stopper.isAlive)
            release.countDown()
            installer.join(3000); stopper.join(3000)
            assertFalse(installer.isAlive); assertFalse(stopper.isAlive)
            failure.get()?.let { throw it }
            assertEquals(listOf("retired"), events)
            assertEquals(1, tun.closes)
            assertEquals(1, process.closes)
        } finally { release.countDown(); installer.join(3000); stopper.join(3000) }
    }

    @Test fun processIsOwnedBeforePublicationCanFail() {
        val f = Fixture()
        val a = f.start(Any())
        val tun = f.gate.acquireTun(a) { Resource() }!!
        val process = Resource()
        try {
            f.gate.acquireProcess(a, installed = { throw IllegalStateException("pump failed") }) { process }
            fail("must propagate")
        } catch (_: IllegalStateException) { f.stop(a) }
        assertEquals(1, tun.closes)
        assertEquals(1, process.closes)
    }

    @Test fun cleanupFailureRetainsOwnerAndHandlesForRetry() {
        var failClose = true
        var failDestroy = true
        val tun = Resource(); val process = Resource()
        val gate = VpnSessionOwnership<Resource, Resource>(
            { if (failClose) error("close failed") else it.closes++ },
            { if (failDestroy) error("destroy failed") else it.closes++ })
        val a = gate.start(Any()) {}!!
        gate.acquireTun(a) { tun }; gate.acquireProcess(a) { process }
        var stopped = false
        try { gate.retire(a) { stopped = true } } catch (_: Exception) {}
        assertFalse("Failed cleanup must not publish stopped", stopped)
        assertSame(tun, a.tun); assertSame(process, a.process)
        try { gate.start(Any()) {}; fail("Must not replace unreleased owner") } catch (_: IllegalStateException) {}
        failClose = false; failDestroy = false
        gate.retire(a) { stopped = true }
        assertTrue(stopped); assertEquals(1, tun.closes); assertEquals(1, process.closes)
    }

    private fun routing(mode: String, selected: Set<String>, installed: Set<String>): Triple<Int, List<String>, List<String>> {
        val allowed = mutableListOf<String>()
        val denied = mutableListOf<String>()
        val count = VpnAppRouting.apply(mode, selected, "self",
            { allowed += it; it in installed }, { denied += it; it == "self" || it in installed })
        return Triple(count, allowed, denied)
    }

    @Test fun missingOnlyAllowlistRejectsBeforeEstablishWithoutAllAppFallback() {
        rejectAllowlist(setOf("missing"))
    }
    @Test fun emptyAllowlistRejectsBeforeEstablish() { rejectAllowlist(emptySet()) }
    @Test fun selfAndBlankOnlyAllowlistRejectsBeforeEstablish() { rejectAllowlist(setOf("self", " ")) }
    private fun rejectAllowlist(selected: Set<String>) {
        val denied = mutableListOf<String>()
        var established = false
        try {
            VpnAppRouting.apply("allowlist", selected, "self", { false }, { denied += it; true })
            established = true // builder.establish is strictly after production routing
            fail("Ineffective allowlist must fail closed")
        } catch (_: IllegalStateException) { /* expected */ }
        assertFalse(established)
        assertTrue("Never turn allowlist into disallow/all-app", denied.isEmpty())
    }
    @Test fun validAndMixedAllowlistsKeepOnlyInstalledPackages() {
        for (selected in listOf(setOf("valid"), setOf("valid", "missing", "self", ""))) {
            val (count, allowed, denied) = routing("allowlist", selected, setOf("valid"))
            assertEquals(1, count)
            assertTrue("valid" in allowed)
            assertTrue(denied.isEmpty())
        }
    }
    @Test fun allAndBypassRetainSelfExclusionAndMissingPackageTolerance() {
        assertEquals(Triple(0, emptyList<String>(), listOf("self")), routing("all", setOf("valid"), setOf("valid")))
        assertEquals(Triple(1, emptyList<String>(), listOf("self", "missing", "valid")), routing("bypass", setOf("valid", "missing"), setOf("valid")))
    }
    @Test fun builderErrorsPropagateInsteadOfEstablishing() {
        try {
            VpnAppRouting.apply("allowlist", setOf("valid"), "self", { throw SecurityException("denied") }, { true })
            fail("must propagate")
        } catch (_: SecurityException) { }
    }
}
