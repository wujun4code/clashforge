package com.clashforge.mobile.clashforge_mobile

import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

class VpnControlRequestsTest {
    @Test fun asyncCloseAndLifecycleKeepControlDeadlineAndOwnership() {
        for (blockClose in listOf(true, false)) {
            val entered = CountDownLatch(1); val release = CountDownLatch(1)
            val cleanup = java.util.concurrent.Executors.newSingleThreadExecutor()
            val controls = java.util.concurrent.Executors.newSingleThreadExecutor()
            val closes = java.util.concurrent.atomic.AtomicInteger()
            val publications = java.util.concurrent.atomic.AtomicInteger()
            val queued = java.util.concurrent.atomic.AtomicInteger()
            val gate = VpnSessionOwnership<Any, Any>({
                closes.incrementAndGet()
                if (blockClose) { entered.countDown(); check(release.await(3, TimeUnit.SECONDS)) }
            }, {})
            val s = gate.start(Any()) {}!!; val tun = Any(); gate.acquireTun(s) { tun }
            val schedule: (() -> Unit) -> Unit = { task -> queued.incrementAndGet(); cleanup.execute(task) }
            val publish = {
                if (!blockClose) { entered.countDown(); check(release.await(3, TimeUnit.SECONDS)) }
                publications.incrementAndGet(); Unit
            }
            try {
                val c = Client(); val id = c.request()
                controls.submit {
                    c.controller.stop(id) {
                        gate.requestRetire(s, publish, schedule)
                        check(gate.awaitRetired(s, 30)) { "VPN cleanup still pending" }
                    }
                }.get(500, TimeUnit.MILLISECONDS)
                assertTrue(entered.await(500, TimeUnit.MILLISECONDS))
                assertEquals(listOf("error:VPN cleanup still pending"), c.replies)
                // A subsequent job on the same control worker must run before release.
                controls.submit { gate.cancel(s) }.get(300, TimeUnit.MILLISECONDS)
                assertSame(s, gate.currentSession()); assertEquals(0, publications.get())
                if (blockClose) assertSame(tun, s.tun)
                repeat(20) { gate.requestRetire(s, publish, schedule) }
                assertEquals(1, queued.get()); assertEquals(1, closes.get())
                try { gate.start(Any(), retirePrevious = false) {}; fail("pending owner replaced") }
                catch (_: IllegalStateException) {}
                val retry = c.request()
                controls.submit {
                    c.controller.stop(retry) {
                        gate.requestRetire(s, publish, schedule)
                        check(gate.awaitRetired(s, 30)) { "VPN cleanup still pending" }
                    }
                }.get(500, TimeUnit.MILLISECONDS)
                assertEquals(2, c.replies.size); assertEquals(1, queued.get())
                release.countDown(); assertTrue(gate.awaitRetired(s, 1000))
                c.controller.stop(id) {}; c.controller.stop(retry) {}
                assertEquals(2, c.replies.size) // no late stopped for timed-out attempts
                assertEquals(1, publications.get()); assertEquals(1, closes.get())
                val fresh = gate.start(Any(), retirePrevious = false) {}!!
                gate.requestRetire(s, publish, schedule)
                assertSame(fresh, gate.currentSession()); assertEquals(1, queued.get())
                gate.retire(fresh) {}
            } finally { release.countDown(); cleanup.shutdownNow(); controls.shutdownNow() }
        }
    }

    @Test fun queuedCleanupRetryClearsFailureBeforeWorkerBegins() {
        var attempts = 0; var stopped = 0
        val gate = VpnSessionOwnership<Any, Any>({ if (++attempts == 1) error("first close") }, {})
        val s = gate.start(Any()) {}!!; val tun = Any(); gate.acquireTun(s) { tun }
        val tasks = mutableListOf<() -> Unit>()
        val schedule: (() -> Unit) -> Unit = { tasks += it }
        gate.requestRetire(s, { stopped++ }, schedule); tasks.removeAt(0)()
        try { gate.awaitRetired(s, 10); fail("must expose failed cleanup") }
        catch (e: IllegalStateException) { assertEquals("first close", e.message) }
        assertSame(tun, s.tun); assertSame(s, gate.currentSession())
        gate.requestRetire(s, { stopped++ }, schedule)
        repeat(20) { gate.requestRetire(s, {}, schedule) }
        assertEquals(1, tasks.size)
        assertFalse("queued retry must not throw old failure", gate.awaitRetired(s, 10))
        tasks.removeAt(0)()
        assertTrue(gate.awaitRetired(s, 100)); assertEquals(2, attempts); assertEquals(1, stopped)
    }

    @Test fun asyncRetirementResumesAfterLateAcquisitionWithoutInlineDisposal() {
        val entered = CountDownLatch(1); val release = CountDownLatch(1)
        val cleanup = java.util.concurrent.Executors.newSingleThreadExecutor()
        val closedOn = AtomicReference<Thread?>()
        val gate = VpnSessionOwnership<Any, Any>({ closedOn.set(Thread.currentThread()) }, {})
        val s = gate.start(Any()) {}!!
        val installer = Thread { gate.acquireTun(s) {
            entered.countDown(); check(release.await(3, TimeUnit.SECONDS)); Any()
        } }
        installer.start()
        try {
            assertTrue(entered.await(500, TimeUnit.MILLISECONDS))
            gate.requestRetire(s, {}, { cleanup.execute(it) })
            assertFalse(gate.awaitRetired(s, 10)); assertSame(s, gate.currentSession())
            release.countDown(); installer.join(1000)
            assertTrue(gate.awaitRetired(s, 1000)); assertNotNull(closedOn.get())
            assertNotSame(installer, closedOn.get())
        } finally { release.countDown(); installer.join(1000); cleanup.shutdownNow() }
    }

    private class Client {
        val controller = VpnControlRequests()
        val replies = mutableListOf<String>()
        lateinit var timeout: () -> Unit
        var cancelled = 0
        fun request(dispatch: (Long) -> Unit = {}): Long = controller.submit(
            { timeout = it; {} }, { cancelled++ }, dispatch, { replies += it })
    }
    @Test fun dispatchIsNotCompletionAndActualRoutingFailureReturnsError() {
        val c = Client(); val id = c.request()
        assertTrue(c.replies.isEmpty())
        assertFalse(c.controller.started(id) {
            VpnAppRouting.apply("allowlist", setOf("uninstalled"), "self", { false }, { true })
        })
        assertEquals(1, c.replies.size); assertTrue(c.replies.single().startsWith("error:"))
        assertFalse(c.controller.started(id) {})
        assertEquals(1, c.replies.size)
    }
    @Test fun readySuccessAndStopAreExactlyOnce() {
        val c = Client(); val id = c.request()
        assertTrue(c.controller.started(id) {})
        c.controller.fail(id, IllegalStateException("late")); c.timeout()
        assertEquals(listOf("started"), c.replies)
        val stop = c.request(); c.controller.stop(stop) {}
        c.controller.stop(stop) {}; assertEquals(listOf("started", "stopped"), c.replies)
        assertEquals(0, c.cancelled)
    }
    @Test fun timeoutCancellationAndLateCompletionDoNotCommit() {
        val c = Client(); val id = c.request()
        c.timeout(); c.timeout()
        assertFalse(c.controller.started(id) {})
        assertEquals(1, c.cancelled); assertEquals(1, c.replies.size)
        assertTrue(c.replies.single().startsWith("error:"))
        val next = c.request(); assertTrue(c.controller.started(next) {})
        assertEquals(2, c.replies.size)
    }
    @Test fun dispatchExceptionAndExplicitCancelAreExactlyOnce() {
        val c = Client(); c.request { error("delivery failed") }
        assertEquals(listOf("error:delivery failed"), c.replies)
        val id = c.request(); c.controller.fail(id, IllegalStateException("cancelled"))
        c.timeout(); c.controller.started(id) {}
        assertEquals(listOf("error:delivery failed", "error:cancelled"), c.replies)
    }
    @Test fun startReadinessFailureNeverReturnsStarted() {
        for (failure in listOf("Invalid TUN", "ConfigGen failed", "spawn failed", "readiness timeout")) {
            val c = Client(); val id = c.request()
            assertFalse(c.controller.started(id) { error(failure) })
            assertEquals(listOf("error:$failure"), c.replies)
        }
    }
    @Test fun realStopControllerRetainsFailedDisposalAndRetries() {
        for (failTun in listOf(true, false)) {
            var failDispose = true
            var tunClosed = 0; var processStopped = 0; var lifecycleStopped = false
            val tun = Any(); val process = Any()
            val gate = VpnSessionOwnership<Any, Any>(
                { if (failTun && failDispose) error("close") else tunClosed++ },
                { if (!failTun && failDispose) error("destroy") else processStopped++ })
            val s = gate.start(Any()) {}!!
            gate.acquireTun(s) { tun }; gate.acquireProcess(s) { process }
            val c = Client(); val stop = c.request()
            c.controller.stop(stop) { gate.retire(s) { lifecycleStopped = true } }
            assertTrue(c.replies.single().startsWith("error:")); assertFalse(lifecycleStopped)
            assertSame(s, gate.currentSession())
            if (failTun) assertSame(tun, s.tun) else assertSame(process, s.process)
            failDispose = false
            val retry = c.request()
            c.controller.stop(retry) { check(gate.retire(s) {}) }
            assertEquals("stopped", c.replies.last()); assertTrue(lifecycleStopped)
            assertNull(gate.currentSession()); assertEquals(1, tunClosed); assertEquals(1, processStopped)
        }
    }
    @Test fun slowLogCallbackDoesNotHoldCancellationGate() {
        val entered = CountDownLatch(1); val release = CountDownLatch(1)
        var closed = 0
        val gate = VpnSessionOwnership<Any, Any>({ closed++ }, { closed++ })
        val s = gate.start(Any()) {}!!; gate.acquireTun(s) { Any() }
        val error = AtomicReference<Throwable?>()
        val worker = Thread {
            try { gate.acquireProcess(s, installed = {
                entered.countDown(); check(release.await(3, TimeUnit.SECONDS))
            }) { Any() } } catch (t: Throwable) { error.set(t) }
        }
        worker.start()
        try {
            assertTrue(entered.await(3, TimeUnit.SECONDS))
            val returned = CountDownLatch(1)
            val stop = Thread { assertFalse(gate.retire(s) {}); returned.countDown() }
            stop.start()
            assertTrue("STOP must return pending while log callback is suspended", returned.await(300, TimeUnit.MILLISECONDS))
            assertFalse(gate.isCurrent(s)); release.countDown(); worker.join(3000); stop.join(3000)
            error.get()?.let { throw it }
            assertTrue(gate.awaitRetired(s, 100)); assertEquals(2, closed)
        } finally { release.countDown(); worker.join(3000) }
    }
    @Test fun slowRetirementPublicationDoesNotBlockCancellationOrPermitReplacement() {
        val entered = CountDownLatch(1); val release = CountDownLatch(1)
        val gate = VpnSessionOwnership<Any, Any>({}, {})
        val s = gate.start(Any()) {}!!
        val worker = Thread { gate.retire(s) {
            entered.countDown(); check(release.await(3, TimeUnit.SECONDS))
        } }
        worker.start()
        try {
            assertTrue(entered.await(3, TimeUnit.SECONDS))
            val cancelled = CountDownLatch(1)
            val cancel = Thread { gate.cancel(s); cancelled.countDown() }
            cancel.start()
            assertTrue(cancelled.await(300, TimeUnit.MILLISECONDS))
            try { gate.start(Any()) {}; fail("Cannot replace pending lifecycle lease") }
            catch (_: IllegalStateException) {}
            release.countDown(); worker.join(3000); cancel.join(3000)
            assertNull(gate.currentSession())
        } finally { release.countDown(); worker.join(3000) }
    }
    @Test fun adoptionBeforeRestorationFailureRetainsProcess() {
        var closed = 0
        val gate = VpnSessionOwnership<Any, Any>({ closed++ }, { closed++ })
        val s = gate.start(Any()) {}!!; gate.acquireTun(s) { Any() }
        val process = Any()
        try { gate.acquireProcess(s) { gate.recordProcess(s, process); error("restore stdin") } }
        catch (_: IllegalStateException) {}
        assertSame(process, s.process); gate.retire(s) {}; assertEquals(2, closed)
    }
    @Test fun activityAndServiceWireRealCompletionsNotDispatchAcknowledgements() {
        // Source boundary assertion complements actual production-controller JVM tests.
        // Android runtime is deliberately not emulated by these tests.
        val root = System.getProperty("vpn.wiring.root") ?: sequenceOf(
            "src/main/kotlin/com/clashforge/mobile/clashforge_mobile",
            "app/src/main/kotlin/com/clashforge/mobile/clashforge_mobile",
            "mobile/android/app/src/main/kotlin/com/clashforge/mobile/clashforge_mobile",
        ).first { File(it, "MainActivity.kt").exists() }
        val prefix = System.getProperty("vpn.wiring.prefix", "")
        val activity = File(root, prefix + "MainActivity.kt").readText()
        val service = File(root, prefix + "ClashVpnService.kt").readText()
        assertFalse("START must not acknowledge intent delivery", activity.contains("result.success(\"started\")"))
        assertFalse("STOP must not acknowledge intent delivery", activity.contains("result.success(\"stopped\")"))
        assertTrue(activity.contains("ClashVpnService.requests.submit("))
        assertTrue(service.contains("requests.started(id)")); assertTrue(service.contains("requests.stop(id)"))
        assertTrue(service.contains("check(ok)")); assertFalse(service.contains("ms — continuing"))
        val stopBody = service.substringAfter("private fun stopSession(").substringBefore("private fun runSession(")
        assertTrue(stopBody.contains("sessions.requestRetire(owned"))
        assertFalse(stopBody.contains("sessions.retire("))
        assertTrue(stopBody.contains("cleanup.execute(it)"))
        assertTrue(stopBody.contains("sessions.awaitRetired(owned, 10_000)"))
        assertTrue(service.contains("sessions.start(this, retirePrevious = false)"))
        assertTrue(service.contains("stopSession(previous)"))
        assertTrue(stopBody.contains("val owner = owned.owner as ClashVpnService"))
        assertTrue(stopBody.contains("owner.stopForeground("))
        assertTrue(stopBody.contains("owner.sendVpnStateBroadcast(false)"))
        assertTrue(stopBody.contains("owner.stopSelfResult(owner.sessionStartId)"))
        assertFalse(service.contains("stopSelf()"))
        assertTrue(service.contains("startVpn(id, epoch, startId)"))
        assertTrue(service.contains("sessionStartId = startId"))
    }
}
