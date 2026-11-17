package com.clashforge.mobile.clashforge_mobile

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ComponentName
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.VpnService
import android.net.NetworkCapabilities
import android.os.Build
import android.os.ParcelFileDescriptor
import android.service.quicksettings.TileService
import android.system.Os
import android.util.Log
import androidx.core.app.NotificationCompat
import java.io.File
import java.io.FileDescriptor
import java.io.FileOutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest

class ClashVpnService : VpnService() {
    @Volatile private var session: VpnSessionOwnership.Session<ParcelFileDescriptor, Process>? = null

    companion object {
        const val ACTION_START = "com.clashforge.mobile.START"
        const val ACTION_STOP  = "com.clashforge.mobile.STOP"
        const val ACTION_VPN_STATE_CHANGED = "com.clashforge.mobile.VPN_STATE_CHANGED"
        const val EXTRA_VPN_RUNNING = "vpn_running"
        private const val TAG        = "ClashVpnService"
        private const val CHANNEL_ID = "clashforge_vpn"
        private const val NOTIF_ID   = 1001
        // Serialize shared runtime-file preparation, without making STOP wait
        // for DNS probing or asset/config I/O under the ownership gate.
        private val startupLock = Any()

        internal val requests = VpnControlRequests()
        const val EXTRA_REQUEST_ID = "control_request_id"

        private val controls = java.util.concurrent.Executors.newSingleThreadExecutor()
        private val cleanup = java.util.concurrent.Executors.newSingleThreadExecutor()
        private val generation = java.util.concurrent.atomic.AtomicLong()
        private val sessions = VpnSessionOwnership<ParcelFileDescriptor, Process>(
            closeTun = { it.close() },
            stopProcess = { process ->
                process.destroy()
                val deadline = System.nanoTime() + 1_500_000_000L
                while (processIsAlive(process) && System.nanoTime() < deadline) Thread.sleep(25)
                if (processIsAlive(process)) {
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                        process.destroyForcibly()
                        check(process.waitFor(1500, java.util.concurrent.TimeUnit.MILLISECONDS)) {
                            "Core did not terminate"
                        }
                    } else {
                        error("Core did not terminate; retained for retry")
                    }
                }
            },
        )
        private fun processIsAlive(process: Process): Boolean = try {
            process.exitValue(); false
        } catch (_: IllegalThreadStateException) { true }
        fun cancelStart(id: Long) {
            val owner = sessions.currentSession()?.owner as? ClashVpnService
            if (owner != null && owner.startRequestId == id) owner.stopVpn(localOnly = true)
        }

        @Volatile var vpnRunning     = false
        @Volatile var mihomoRunning  = false
        @Volatile var mihomoPid      = -1
    }

    override fun onCreate() {
        super.onCreate()
        controls.execute {
            NativeLogStore.init(applicationContext)
            LogEventBridge.info("vpn", "Service onCreate")
        }
    }

    @Volatile private var startRequestId: Long? = null
    @Volatile private var sessionStartId = 0

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val id = intent?.takeIf { it.hasExtra(EXTRA_REQUEST_ID) }?.getLongExtra(EXTRA_REQUEST_ID, 0)
        when (intent?.action) {
            ACTION_START -> {
                val epoch = generation.incrementAndGet()
                // Bind an existing live lease to this delivered command on the
                // main thread, before a later STOP can install its newer startId.
                sessions.currentSession()?.let { owned ->
                    sessions.withCurrent(owned) {
                        if (owned.owner === this) sessionStartId = startId
                    }
                }
                controls.execute { startVpn(id, epoch, startId) }
            }
            ACTION_STOP -> {
                // Bind this STOP to its old lease, not to a later queued START.
                (sessions.currentSession()?.owner as? ClashVpnService)?.sessionStartId = startId
                stopVpn(id)
            }

        }
        return START_STICKY
    }

    private fun startVpn(id: Long? = null, epoch: Long, startId: Int) {
        if (epoch != generation.get()) {
            requests.fail(id, IllegalStateException("VPN startup superseded"))
            return
        }
        if (id != null && !requests.isPending(id)) return
        var starting: VpnSessionOwnership.Session<ParcelFileDescriptor, Process>? = null
        try {
            sessions.currentSession()?.let { previous ->
                if (previous.owner === this && sessions.isCurrent(previous)) {
                    // Permission completion can start the service without a request
                    // ID. A later Flutter START joins only this proven healthy lease.
                    // Network I/O stays outside the ownership monitor.
                    waitForMihomoReady(previous)
                    val committed = sessions.withCurrent(previous) {
                        check(epoch == generation.get()) { "VPN startup superseded" }
                        requests.started(id) {
                            check(previous.process?.let { processIsAlive(it) } == true &&
                                previous.tun?.fileDescriptor?.valid() == true) {
                                "Core or VPN interface lost before startup completion"
                            }
                        }
                    } ?: false
                    if (!committed) requests.fail(id, IllegalStateException("VPN startup cancelled"))
                    // Do not replace the primary request ID: a duplicate waiter's
                    // timeout must never cancel the healthy original session.
                    return
                }
                stopSession(previous)
            }
            check(epoch == generation.get()) { "VPN startup cancelled" }
            if (id != null && !requests.isPending(id)) return
            val started = sessions.start(this, retirePrevious = false) { installed ->
                starting = installed
                session = installed
                sessionStartId = startId
                startRequestId = id
                check(epoch == generation.get()) { "VPN startup cancelled" }
                vpnRunning = true // ownership, including a failed-cleanup Disconnect
                mihomoRunning = false
                mihomoPid = -1
            } ?: run {
                requests.fail(id, IllegalStateException("VPN already starting or running"))
                return
            }
            if (id != null && !requests.isPending(id)) {
                stopSession(started)
                return
            }
            createNotificationChannel()
            val notif = buildVpnNotification()
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                startForeground(NOTIF_ID, notif, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
            } else {
                startForeground(NOTIF_ID, notif)
            }
            Thread({ runSession(started, id) }, "ClashVpnThread").start()
        } catch (e: Exception) {
            requests.fail(id, e)
            starting?.let { owned ->
                try { stopSession(owned) } catch (cleanup: Exception) {
                    Log.e(TAG, "Startup cleanup failed; owner retained", cleanup)
                }
            }
        }
    }

    private fun stopVpn(id: Long? = null, localOnly: Boolean = false) {
        // Only cancellation touches the main callback. Disk/process close happens off-thread.
        if (!localOnly) generation.incrementAndGet()
        val owned = if (localOnly) session else sessions.currentSession()
        owned?.let { sessions.cancel(it) }
        requests.fail((owned?.owner as? ClashVpnService)?.startRequestId,
            IllegalStateException("VPN startup cancelled"))
        controls.execute {
            val target = if (localOnly) owned else sessions.currentSession()
            requests.stop(id) { target?.let { stopSession(it) } }
        }
    }

    private fun stopSession(owned: VpnSessionOwnership.Session<ParcelFileDescriptor, Process>) {
        val owner = owned.owner as ClashVpnService
        sessions.requestRetire(owned, {
            @Suppress("DEPRECATION")
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                owner.stopForeground(Service.STOP_FOREGROUND_REMOVE)
            } else {
                owner.stopForeground(true)
            }
            owner.sendVpnStateBroadcast(false)
            // Android rejects this stop if a newer START was already delivered,
            // including when its control worker is still queued behind retirement.
            owner.stopSelfResult(owner.sessionStartId)
            vpnRunning = false
            mihomoRunning = false
            mihomoPid = -1
        }, { cleanup.execute(it) })
        check(sessions.awaitRetired(owned, 10_000)) { "VPN cleanup still pending" }
    }

    private fun runSession(owned: VpnSessionOwnership.Session<ParcelFileDescriptor, Process>, id: Long?) {
        try {
            val process = synchronized(startupLock) {
                if (!sessions.isCurrent(owned)) return
                extractAssetsIfNeeded()

                val configFile = File(filesDir, "config.yaml")
                val isCiMode = shouldUseCiTorConfig()
                LogEventBridge.info("vpn", "VPN run starting", mapOf(
                    "ciMode" to isCiMode,
                    "config_exists" to configFile.exists(),
                    "config_size" to configFile.length(),
                    "sdk" to Build.VERSION.SDK_INT,
                    "target_sdk" to applicationInfo.targetSdkVersion,
                ))
                logConnectivitySnapshot("before_establish")

                // ── Step 1: DNS probe (before VPN interface is established) ─────────
                // Probe sockets travel the physical network here.  Once the VPN
                // tunnel is up they would be captured by the TUN and loop back,
                // yielding false negatives.
                if (!isCiMode && configFile.exists()) {
                    val probe = ConfigGen.probeAndPatchDNS(configFile.absolutePath)
                    LogEventBridge.info("dns", probe.summary)
                }
                if (!sessions.isCurrent(owned)) return

                // ── Step 2: Establish VPN interface ──────────────────────────────────
                LogEventBridge.info("vpn", "Building VPN interface",
                    mapOf("addr" to "172.19.0.1/30", "route" to "0.0.0.0/0", "dns" to "172.19.0.2"))

                val builder = Builder()
                    .addAddress("172.19.0.1", 30)
                    .addRoute("0.0.0.0", 0)
                    // DNS must be the /30 peer address (172.19.0.2), not the interface's
                    // own address (172.19.0.1).  The kernel's local-table intercepts
                    // packets to the interface's own IP before any VPN routing applies,
                    // so queries to 172.19.0.1:53 never reach the TUN fd.  172.19.0.2
                    // is forwarded through tun0 → gvisor → dns-hijack as expected.
                    .addDnsServer("172.19.0.2")
                    .setMtu(1500)
                    .setSession("ClashForge")
                    .setBlocking(false)

                // VPNs default to metered for apps targeting Android Q+. Passing
                // false does not force the VPN to be unmetered; Android then
                // inherits meteredness from the underlying network. This keeps the
                // VPN unmetered on unmetered Wi-Fi, but Play Store jobs requiring
                // NOT_METERED may still wait forever on metered cellular data.
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    builder.setMetered(false)
                    LogEventBridge.info("vpn", "Applied VPN metered policy", mapOf(
                        "set_metered" to false,
                        "behavior" to "inherit_underlying_meteredness",
                    ))
                }

                applyAppRouting(builder)

                val tun = sessions.acquireTun(owned) { builder.establish() } ?: run {
                    error("VPN interface unavailable or startup cancelled")
                }
                val tunFd = tun.fd
                check(tunFd >= 0 && tun.fileDescriptor.valid()) { "Invalid VPN interface" }
                LogEventBridge.info("vpn", "VPN interface established", mapOf("fd" to tunFd))
                logConnectivitySnapshot("after_establish")

                // ── Step 3: Core apply — write final config.yaml ─────────────────────
                // tunFd argument is always 0: startMihomoCore dup2s the real fd onto
                // stdin before forking, so file-descriptor: 0 in the YAML means fd 0.
                //
                // dns_mode is written by the Flutter UI (SharedPreferences key
                // "flutter.dns_mode") when the user chooses fake-ip or redir-host.
                val dnsMode = getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
                    .getString("flutter.dns_mode", "fake-ip") ?: "fake-ip"
                val powerProfile = getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
                    .getString("flutter.power_profile", "balanced")
                    ?.takeIf { it == "battery" || it == "performance" }
                    ?: "balanced"

                if (!sessions.isCurrent(owned)) return
                if (isCiMode) {
                    configFile.writeText(buildCiConfig(0))
                    LogEventBridge.info("vpn", "CI mode: wrote Tor config", mapOf("tunFd" to 0))
                } else if (!configFile.exists()) {
                    error("config.yaml not found")
                } else {
                    val geoDataDir = filesDir.absolutePath
                    val ok = ConfigGen.generateConfig(
                        configFile.absolutePath, 0, geoDataDir, dnsMode, powerProfile,
                    )
                    check(ok) { "Core config generation failed" }
                    LogEventBridge.info("vpn", "Core apply complete",
                        mapOf("ok" to ok, "dnsMode" to dnsMode, "powerProfile" to powerProfile,
                            "stack" to "gvisor", "note" to "stdin(fd0)=TUN"))
                    logRuntimeConfigSummary(configFile, ok, dnsMode)
                }

                // ── Step 4: Start mihomo core ─────────────────────────────────────────
                val process = sessions.acquireProcess(owned,
                    installed = { startCoreLogPump(owned, it, tunFd) },
                ) { ownedTun ->
                    startMihomoCore(owned, ownedTun)
                } ?: error("Core spawn failed or startup cancelled")
                process
            }

            // ── Step 5: Wait for mihomo to be ready ───────────────────────────────
            // Prevents Android ConnectivityManager from marking the VPN as "no
            // internet" during the ~1-2 s mihomo needs to bind its DNS server and
            // start intercepting packets from the TUN device.
            waitForMihomoReady(owned)
            var committed = false
            sessions.duringSession(owned) {
                committed = sessions.withCurrent(owned) {
                    requests.started(id) {
                        check(processIsAlive(process) && owned.tun?.fileDescriptor?.valid() == true) {
                            "Core or VPN interface lost before startup completion"
                        }
                    }
                } ?: false
                if (committed) sendVpnStateBroadcast(true)
            }
            if (!committed) return
            logConnectivitySnapshot("after_core_ready")

            LogEventBridge.info("vpn", "VPN core running")
            val exitCode = process.waitFor()
            sessions.withCurrent(owned) {
                mihomoRunning = false
                mihomoPid = -1
            }
            LogEventBridge.warn("mihomo", "Core exited unexpectedly",
                mapOf("exit_code" to exitCode))

        } catch (e: Exception) {
            requests.fail(id, e)
            Log.e(TAG, "VPN run error", e)
            LogEventBridge.error("vpn", "VPN run() exception: ${e.message}",
                mapOf("type" to e.javaClass.simpleName))
        } finally {
            requests.fail(id, IllegalStateException("VPN startup cancelled"))
            try { if (sessions.isCurrent(owned)) stopSession(owned) } catch (e: Exception) {
                Log.e(TAG, "VPN cleanup failed; owner retained for Disconnect retry", e)
            }
        }
    }

    private fun logConnectivitySnapshot(reason: String) {
        try {
            val cm = getSystemService(CONNECTIVITY_SERVICE) as ConnectivityManager
            val activeNetwork = cm.activeNetwork
            val activeCaps = activeNetwork?.let { cm.getNetworkCapabilities(it) }
            val activeLp = activeNetwork?.let { cm.getLinkProperties(it) }
            val vpnNetwork = cm.allNetworks.firstOrNull { network ->
                cm.getNetworkCapabilities(network)
                    ?.hasTransport(NetworkCapabilities.TRANSPORT_VPN) == true
            }
            val vpnCaps = vpnNetwork?.let { cm.getNetworkCapabilities(it) }
            val vpnLp = vpnNetwork?.let { cm.getLinkProperties(it) }
            val vpnNotMetered =
                vpnCaps?.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED) == true

            LogEventBridge.info("net", "Connectivity snapshot", mapOf(
                "reason" to reason,
                "active_metered" to cm.isActiveNetworkMetered,
                "active_transports" to describeTransports(activeCaps),
                "active_caps" to describeCapabilities(activeCaps),
                "active_if" to (activeLp?.interfaceName ?: ""),
                "vpn_present" to (vpnCaps != null),
                "vpn_metered" to (vpnCaps != null && !vpnNotMetered),
                "vpn_not_metered" to vpnNotMetered,
                "vpn_transports" to describeTransports(vpnCaps),
                "vpn_caps" to describeCapabilities(vpnCaps),
                "vpn_if" to (vpnLp?.interfaceName ?: ""),
                "vpn_dns" to (vpnLp?.dnsServers?.joinToString(",") ?: ""),
                "vpn_mtu" to (vpnLp?.mtu ?: 0),
            ))
        } catch (e: Exception) {
            LogEventBridge.warn("net", "Connectivity snapshot failed",
                mapOf("reason" to reason, "error" to (e.message ?: e.javaClass.simpleName)))
        }
    }

    private fun logRuntimeConfigSummary(configFile: File, ok: Boolean, dnsMode: String) {
        try {
            val text = configFile.readText()
            val lines = text.lines()
            LogEventBridge.info("config", "Runtime config summary", mapOf(
                "ok" to ok,
                "dnsMode" to dnsMode,
                "size" to configFile.length(),
                "sha256_12" to sha256(text).take(12),
                "dns_enhanced_mode" to findScalarInSection(lines, "dns", "enhanced-mode"),
                "dns_respect_rules" to findScalarInSection(lines, "dns", "respect-rules"),
                "fake_ip_range" to findScalarInSection(lines, "dns", "fake-ip-range"),
                "tun_stack" to findScalarInSection(lines, "tun", "stack"),
                "tun_mtu" to findScalarInSection(lines, "tun", "mtu"),
                "tun_auto_route" to findScalarInSection(lines, "tun", "auto-route"),
                "sniffer_override_destination" to
                    findScalarInSection(lines, "sniffer", "override-destination"),
            ))
        } catch (e: Exception) {
            LogEventBridge.warn("config", "Runtime config summary failed",
                mapOf("error" to (e.message ?: e.javaClass.simpleName)))
        }
    }

    private fun findScalarInSection(lines: List<String>, section: String, key: String): String {
        var inSection = false
        val sectionHeader = "$section:"
        val keyPrefix = "$key:"
        for (line in lines) {
            val trimmed = line.trim()
            if (!line.startsWith(" ") && !line.startsWith("-")) {
                inSection = trimmed == sectionHeader
                continue
            }
            if (inSection && trimmed.startsWith(keyPrefix)) {
                return trimmed.substringAfter(":").trim().take(96)
            }
        }
        return ""
    }

    private fun sha256(text: String): String {
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(text.toByteArray(Charsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it.toInt() and 0xff) }
    }

    private fun describeTransports(caps: NetworkCapabilities?): String {
        if (caps == null) return ""
        val values = mutableListOf<String>()
        if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) values += "CELLULAR"
        if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) values += "WIFI"
        if (caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) values += "VPN"
        if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) values += "ETHERNET"
        if (caps.hasTransport(NetworkCapabilities.TRANSPORT_BLUETOOTH)) values += "BLUETOOTH"
        if (caps.hasTransport(NetworkCapabilities.TRANSPORT_USB)) values += "USB"
        return values.joinToString("|").ifEmpty { "unknown" }
    }

    private fun describeCapabilities(caps: NetworkCapabilities?): String {
        if (caps == null) return ""
        val values = mutableListOf<String>()
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)) values += "INTERNET"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)) values += "VALIDATED"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_TRUSTED)) values += "TRUSTED"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)) values += "NOT_METERED"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_RESTRICTED)) values += "NOT_RESTRICTED"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)) values += "NOT_VPN"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_FOREGROUND)) values += "FOREGROUND"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_CONGESTED)) values += "NOT_CONGESTED"
        if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_SUSPENDED)) values += "NOT_SUSPENDED"
        return values.joinToString("|").ifEmpty { "none" }
    }

    // ── CI / Tor bypass ───────────────────────────────────────────────────────

    // Returns true only when CI Tor mode is explicitly enabled via a marker file
    // and the forwarded Tor SOCKS5 endpoint is reachable.
    // Marker: /data/local/tmp/clashforge_ci_tor.enable
    private fun shouldUseCiTorConfig(): Boolean {
        val marker = File("/data/local/tmp/clashforge_ci_tor.enable")
        if (!marker.exists()) return false
        return isTorPortReachable()
    }

    private fun isTorPortReachable(): Boolean = try {
        java.net.Socket().use {
            it.connect(java.net.InetSocketAddress("127.0.0.1", 9050), 300)
        }
        true
    } catch (_: Exception) { false }

    // In CI, subscription proxy nodes are blocked from GitHub Actions (Azure) IPs.
    // When Tor is detected on 127.0.0.1:9050 (forwarded via adb reverse), we
    // replace the entire config with a minimal one that routes through Tor.
    private fun buildCiConfig(tunFd: Int) = """
port: 7890
socks-port: 7891
allow-lan: false
mode: rule
log-level: debug
external-controller: 127.0.0.1:9090

dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  nameserver:
    - 8.8.8.8
    - 1.1.1.1
  fake-ip-filter:
    - "*.lan"

proxies:
  - name: ci-tor
    type: socks5
    server: 127.0.0.1
    port: 9050

proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - ci-tor

rules:
  - GEOIP,private,DIRECT,no-resolve
  - MATCH,Proxy

tun:
  enable: true
  stack: gvisor
  file-descriptor: $tunFd
  auto-route: false
  auto-detect-interface: false
  dns-hijack:
    - "any:53"
    - "tcp://any:53"
    - "tls://any:853"

sniffer:
  enable: true
  override-destination: true
  parse-pure-ip: true
  sniff:
    TLS:
      ports: [443, 8443]
    HTTP:
      ports: [80, 8080-8880]
    QUIC:
      ports: [443]
""".trimIndent()

    // ── Readiness probe ───────────────────────────────────────────────────────

    private fun waitForMihomoReady(
        owned: VpnSessionOwnership.Session<ParcelFileDescriptor, Process>,
        timeoutMs: Long = 5000L,
    ) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline && sessions.isCurrent(owned)) {
            try {
                val conn = URL("http://127.0.0.1:9090/version").openConnection()
                    as HttpURLConnection
                conn.connectTimeout = 300
                conn.readTimeout = 300
                conn.requestMethod = "GET"
                val code = conn.responseCode
                conn.disconnect()
                if (code == HttpURLConnection.HTTP_OK) {
                    LogEventBridge.info("mihomo", "Core API ready")
                    return
                }
            } catch (_: Exception) {}
            Thread.sleep(200)
        }
        error("Core API not ready within ${timeoutMs}ms or startup cancelled")
    }

    // ── Mihomo process ────────────────────────────────────────────────────────

    private fun startMihomoCore(
        owned: VpnSessionOwnership.Session<ParcelFileDescriptor, Process>,
        tun: ParcelFileDescriptor,
    ): Process? {
        val tunFd = tun.fd
        val appDir     = filesDir.absolutePath
        // nativeLibraryDir is on an executable partition; filesDir is mounted
        // noexec on API 29+, so we cannot execute binaries placed there.
        val coreBin    = File(applicationInfo.nativeLibraryDir, "libmihomo.so")
        val configFile = File(appDir, "config.yaml")

        if (!coreBin.exists()) {
            LogEventBridge.error("mihomo", "Binary not found", mapOf("path" to coreBin.absolutePath))
            return null
        }
        coreBin.setExecutable(true)

        LogEventBridge.info("mihomo", "Starting core", mapOf(
            "bin"           to coreBin.absolutePath,
            "bin_size_mb"   to String.format("%.1f", coreBin.length() / 1048576.0),
            "config_exists" to configFile.exists(),
            "config_size"   to configFile.length(),
            "abi"           to (android.os.Build.SUPPORTED_ABIS.firstOrNull() ?: "unknown"),
            "tun_fd"        to tunFd,
        ))

        val tunFdObj = tun.fileDescriptor

        // Android closes ALL fds >= 3 in the child before exec (closeDescriptors).
        // Only fds 0/1/2 survive.  Strategy: dup the TUN fd onto fd 0 (stdin) in
        // THIS process, tell ProcessBuilder to INHERIT stdin so the child gets
        // fd 0 = TUN fd, then restore our own stdin.
        // POSIX dup2() clears FD_CLOEXEC on the new fd so fd 0 survives exec.
        // sing-tun is patched so file-descriptor: 0 means "use fd 0" rather than
        // "not set" (the original sentinel was ==0; we changed it to <0).
        var spawned: Process? = null
        val savedStdin: FileDescriptor = Os.dup(FileDescriptor.`in`)
        try {
            Os.dup2(tunFdObj, 0)   // fd 0 in this process = TUN fd

            val pb = ProcessBuilder(coreBin.absolutePath, "-d", appDir, "-f", configFile.absolutePath)
            pb.environment().apply {
                // Return freed pages to the kernel immediately (not lazily via MADV_FREE).
                put("GODEBUG",    "madvdontneed=1")
                val profile = getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
                    .getString("flutter.power_profile", "balanced")
                when (profile) {
                    "battery" -> {
                        put("GOGC", "100")
                        put("GOMEMLIMIT", "160MiB")
                    }
                    "performance" -> {
                        put("GOGC", "80")
                        put("GOMEMLIMIT", "256MiB")
                    }
                    else -> {
                        put("GOGC", "100")
                        put("GOMEMLIMIT", "192MiB")
                    }
                }
            }
            pb.redirectInput(ProcessBuilder.Redirect.INHERIT)  // fd 0 = TUN
            pb.redirectErrorStream(true)                        // merge stderr into stdout
            spawned = pb.start().also { sessions.recordProcess(owned, it) }
        } finally {
            try {
                Os.dup2(savedStdin, 0)   // restore parent's stdin unconditionally
            } catch (e: Exception) {
                // Already adopted: retirement must retain the handle if destroy fails.
                throw e
            } finally {
                try { Os.close(savedStdin) } catch (e: Exception) {
                    throw e
                }
            }
        }

        return spawned
    }

    private fun startCoreLogPump(
        owned: VpnSessionOwnership.Session<ParcelFileDescriptor, Process>,
        process: Process,
        tunFd: Int,
    ) {
        val pid = try {
            val f = process.javaClass.getDeclaredField("pid")
            f.isAccessible = true
            f.getInt(process)
        } catch (_: Exception) { -1 }

        sessions.withCurrent(owned) {
            mihomoPid = pid
            mihomoRunning = true
        }
        LogEventBridge.info("mihomo", "Core started", mapOf("pid" to pid, "tun_fd" to tunFd))

        Thread {
            try {
                process.inputStream.bufferedReader().useLines { lines ->
                    lines.forEach { line ->
                        LogEventBridge.coreLine(line)
                    }
                }
            } catch (e: Exception) {
                if (sessions.isCurrent(owned)) {
                    LogEventBridge.error("mihomo", "Log reader error: ${e.message}")
                }
            }
        }.apply {
            name = "MihomoLogPump"
            isDaemon = true
            start()
        }
    }

    private fun applyAppRouting(builder: Builder) {
        val prefs = getSharedPreferences("vpn_routing", MODE_PRIVATE)
        val mode = prefs.getString("mode", "all") ?: "all"
        val packages = prefs.getStringSet("packages", emptySet()).orEmpty()
        fun add(appPackage: String, allowed: Boolean): Boolean = try {
            if (allowed) builder.addAllowedApplication(appPackage)
            else builder.addDisallowedApplication(appPackage)
            true
        } catch (_: PackageManager.NameNotFoundException) {
            LogEventBridge.warn("vpn", "Routing app is no longer installed",
                mapOf("package" to appPackage, "allowed" to allowed))
            false
        }
        // Deliberately propagate ineffective allowlists and builder failures:
        // establishing without a successful allow entry would capture every app.
        val applied = VpnAppRouting.apply(mode, packages, packageName,
            allow = { add(it, true) }, disallow = { add(it, false) })
        LogEventBridge.info("vpn", "Applied VPN app routing",
            mapOf("mode" to mode, "count" to applied))
    }

    // ── Asset extraction ──────────────────────────────────────────────────────

    private fun extractAssetsIfNeeded() {
        val markerFile = File(filesDir, ".asset_version")
        val currentVersion = packageManager
            .getPackageInfo(packageName, 0)
            .let {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P)
                    it.longVersionCode.toString()
                else
                    @Suppress("DEPRECATION") it.versionCode.toString()
            }

        if (markerFile.exists() && markerFile.readText().trim() == currentVersion) {
            LogEventBridge.debug("assets", "Assets up-to-date", mapOf("version" to currentVersion))
            return
        }

        LogEventBridge.info("assets", "Extracting assets", mapOf("version" to currentVersion))
        extractAsset("flutter_assets/assets/geodata/country.mmdb", File(filesDir, "country.mmdb"))
        extractAsset("flutter_assets/assets/geodata/geosite.dat",  File(filesDir, "geosite.dat"))

        // Verify extracted files are non-empty so cfgen's geosite:cn filter
        // won't silently fall back to an incomplete dataset.
        for (name in listOf("geosite.dat", "country.mmdb")) {
            val f = File(filesDir, name)
            if (!f.exists() || f.length() == 0L) {
                LogEventBridge.error("assets", "Extracted $name is missing or empty — " +
                    "geosite:cn filter will be skipped this session")
            }
        }

        markerFile.writeText(currentVersion)
        LogEventBridge.info("assets", "Extraction complete")
    }

    private fun extractAsset(assetPath: String, dest: File, executable: Boolean = false) {
        try {
            assets.open(assetPath).use { input ->
                FileOutputStream(dest).use { output -> input.copyTo(output) }
            }
            if (executable) dest.setExecutable(true)
            LogEventBridge.debug("assets", "Extracted ${dest.name}",
                mapOf("size" to dest.length(), "path" to dest.absolutePath))
        } catch (e: Exception) {
            Log.e(TAG, "Failed to extract $assetPath", e)
            LogEventBridge.error("assets", "Extract failed: ${dest.name}",
                mapOf("asset" to assetPath, "error" to (e.message ?: "unknown")))
        }
    }

    // ── Notification ──────────────────────────────────────────────────────────

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "ClashForge",
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply {
                description = "畅行服务连接状态"
                setShowBadge(false)
                setSound(null, null)
                enableVibration(false)
            }
            (getSystemService(NOTIFICATION_SERVICE) as NotificationManager)
                .createNotificationChannel(channel)
        }
    }

    private fun buildVpnNotification(): Notification {
        val stopIntent = PendingIntent.getService(
            this, 0,
            Intent(this, ClashVpnService::class.java).apply { action = ACTION_STOP },
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val openIntent = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("ClashForge 畅行已开启")
            .setContentText("服务已接管网络流量")
            .setSmallIcon(R.drawable.ic_vpn_notification)
            .setContentIntent(openIntent)
            .addAction(0, "断开连接", stopIntent)
            .setOngoing(true)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .build()
    }

    private fun sendVpnStateBroadcast(running: Boolean) {
        sendBroadcast(Intent(ACTION_VPN_STATE_CHANGED).putExtra(EXTRA_VPN_RUNNING, running))
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            TileService.requestListeningState(
                this,
                ComponentName(this, ClashForgeTileService::class.java),
            )
        }
        if (running && Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            try {
                val sbm = getSystemService(android.app.StatusBarManager::class.java)
                sbm?.requestAddTileService(
                    ComponentName(this, ClashForgeTileService::class.java),
                    "ClashForge 畅行",
                    android.graphics.drawable.Icon.createWithResource(
                        this, R.drawable.ic_vpn_notification,
                    ),
                    mainExecutor,
                ) { /* result ignored */ }
            } catch (_: Exception) { /* non-fatal on some OEMs */ }
        }
    }

    // ── Lifecycle ─────────────────────────────────────────────────────────────

    override fun onRevoke() {
        stopVpn(localOnly = true)
        super.onRevoke()
    }

    override fun onDestroy() {
        stopVpn(localOnly = true)
        super.onDestroy()
    }
}
