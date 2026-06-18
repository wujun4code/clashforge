package com.clashforge.mobile.clashforge_mobile

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ComponentName
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
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

class ClashVpnService : VpnService(), Runnable {
    private var vpnThread: Thread? = null
    private var vpnInterface: ParcelFileDescriptor? = null
    private var isRunning = false
    private var coreProcess: Process? = null

    companion object {
        const val ACTION_START = "com.clashforge.mobile.START"
        const val ACTION_STOP  = "com.clashforge.mobile.STOP"
        const val ACTION_VPN_STATE_CHANGED = "com.clashforge.mobile.VPN_STATE_CHANGED"
        const val EXTRA_VPN_RUNNING = "vpn_running"
        private const val TAG        = "ClashVpnService"
        private const val CHANNEL_ID = "clashforge_vpn"
        private const val NOTIF_ID   = 1001

        @Volatile var vpnRunning     = false
        @Volatile var mihomoRunning  = false
        @Volatile var mihomoPid      = -1
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> startVpn()
            ACTION_STOP  -> stopVpn()
        }
        return START_STICKY
    }

    @Synchronized
    private fun startVpn() {
        if (isRunning) {
            LogEventBridge.warn("vpn", "startVpn called but already running")
            return
        }
        isRunning  = true
        vpnRunning = true
        createNotificationChannel()
        val notif = buildVpnNotification()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(NOTIF_ID, notif, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIF_ID, notif)
        }
        sendVpnStateBroadcast(true)
        LogEventBridge.info("vpn", "Starting VPN thread")
        vpnThread = Thread(this, "ClashVpnThread").apply { start() }
    }

    @Synchronized
    private fun stopVpn() {
        if (!isRunning) {
            LogEventBridge.warn("vpn", "stopVpn called but not running")
            return
        }
        isRunning     = false
        vpnRunning    = false
        mihomoRunning = false
        mihomoPid     = -1

        @Suppress("DEPRECATION")
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            stopForeground(Service.STOP_FOREGROUND_REMOVE)
        } else {
            stopForeground(true)
        }
        sendVpnStateBroadcast(false)

        coreProcess?.destroy()
        coreProcess = null
        LogEventBridge.debug("vpn", "Mihomo process destroyed")

        try {
            vpnInterface?.close()
        } catch (e: Exception) {
            Log.e(TAG, "Error closing VPN interface", e)
            LogEventBridge.error("vpn", "Error closing VPN interface: ${e.message}")
        }
        vpnInterface = null
        stopSelf()
        LogEventBridge.info("vpn", "VPN stopped")
    }

    override fun run() {
        try {
            extractAssetsIfNeeded()

            val configFile = File(filesDir, "config.yaml")
            val isCiMode = shouldUseCiTorConfig()

            // ── Step 1: DNS probe (before VPN interface is established) ─────────
            // Probe sockets travel the physical network here.  Once the VPN
            // tunnel is up they would be captured by the TUN and loop back,
            // yielding false negatives.
            if (!isCiMode && configFile.exists()) {
                val probe = ConfigGen.probeAndPatchDNS(configFile.absolutePath)
                LogEventBridge.info("dns", probe.summary)
            }

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

            try {
                builder.addDisallowedApplication(packageName)
                LogEventBridge.info("vpn", "Excluded app from VPN routing",
                    mapOf("package" to packageName))
            } catch (e: Exception) {
                LogEventBridge.warn("vpn", "Failed to exclude app from VPN routing",
                    mapOf("error" to (e.message ?: "unknown")))
            }

            vpnInterface = builder.establish() ?: run {
                LogEventBridge.error("vpn", "builder.establish() returned null")
                return
            }
            val tunFd = vpnInterface!!.fd
            LogEventBridge.info("vpn", "VPN interface established", mapOf("fd" to tunFd))

            // ── Step 3: Core apply — write final config.yaml ─────────────────────
            // tunFd argument is always 0: startMihomoCore dup2s the real fd onto
            // stdin before forking, so file-descriptor: 0 in the YAML means fd 0.
            //
            // dns_mode is written by the Flutter UI (SharedPreferences key
            // "flutter.dns_mode") when the user chooses fake-ip or redir-host.
            val dnsMode = getSharedPreferences("FlutterSharedPreferences", MODE_PRIVATE)
                .getString("flutter.dns_mode", "fake-ip") ?: "fake-ip"

            if (isCiMode) {
                configFile.writeText(buildCiConfig(0))
                LogEventBridge.info("vpn", "CI mode: wrote Tor config", mapOf("tunFd" to 0))
            } else if (!configFile.exists()) {
                LogEventBridge.warn("vpn", "config.yaml not found — skipping core apply")
            } else {
                val geoDataDir = filesDir.absolutePath
                val ok = ConfigGen.generateConfig(configFile.absolutePath, 0, geoDataDir, dnsMode)
                LogEventBridge.info("vpn", "Core apply complete",
                    mapOf("ok" to ok, "dnsMode" to dnsMode, "stack" to "gvisor", "note" to "stdin(fd0)=TUN"))
            }

            // ── Step 4: Start mihomo core ─────────────────────────────────────────
            startMihomoCore(tunFd)

            // ── Step 5: Wait for mihomo to be ready ───────────────────────────────
            // Prevents Android ConnectivityManager from marking the VPN as "no
            // internet" during the ~1-2 s mihomo needs to bind its DNS server and
            // start intercepting packets from the TUN device.
            waitForMihomoReady()

            LogEventBridge.info("vpn", "VPN loop running")
            while (isRunning) Thread.sleep(1000)

        } catch (e: Exception) {
            Log.e(TAG, "VPN run error", e)
            LogEventBridge.error("vpn", "VPN run() exception: ${e.message}",
                mapOf("type" to e.javaClass.simpleName))
        } finally {
            stopVpn()
        }
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

    private fun waitForMihomoReady(timeoutMs: Long = 5000L) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline && isRunning) {
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
        LogEventBridge.warn("mihomo", "Core API not ready within ${timeoutMs}ms — continuing")
    }

    // ── Mihomo process ────────────────────────────────────────────────────────

    private fun startMihomoCore(tunFd: Int) {
        val appDir     = filesDir.absolutePath
        // nativeLibraryDir is on an executable partition; filesDir is mounted
        // noexec on API 29+, so we cannot execute binaries placed there.
        val coreBin    = File(applicationInfo.nativeLibraryDir, "libmihomo.so")
        val configFile = File(appDir, "config.yaml")

        if (!coreBin.exists()) {
            LogEventBridge.error("mihomo", "Binary not found", mapOf("path" to coreBin.absolutePath))
            return
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

        val tunFdObj = vpnInterface?.fileDescriptor ?: run {
            LogEventBridge.error("mihomo", "vpnInterface null before spawn")
            return
        }

        // Android closes ALL fds >= 3 in the child before exec (closeDescriptors).
        // Only fds 0/1/2 survive.  Strategy: dup the TUN fd onto fd 0 (stdin) in
        // THIS process, tell ProcessBuilder to INHERIT stdin so the child gets
        // fd 0 = TUN fd, then restore our own stdin.
        // POSIX dup2() clears FD_CLOEXEC on the new fd so fd 0 survives exec.
        // sing-tun is patched so file-descriptor: 0 means "use fd 0" rather than
        // "not set" (the original sentinel was ==0; we changed it to <0).
        val savedStdin: FileDescriptor = Os.dup(FileDescriptor.`in`)
        try {
            Os.dup2(tunFdObj, 0)   // fd 0 in this process = TUN fd

            val pb = ProcessBuilder(coreBin.absolutePath, "-d", appDir, "-f", configFile.absolutePath)
            pb.environment().apply {
                // Return freed pages to the kernel immediately (not lazily via MADV_FREE).
                put("GODEBUG",    "madvdontneed=1")
                // GC at 40 % heap growth — reduces peak live-object dirty pages.
                put("GOGC",       "40")
                // Soft memory cap; runtime scavenges aggressively to stay under.
                put("GOMEMLIMIT", "100MiB")
            }
            pb.redirectInput(ProcessBuilder.Redirect.INHERIT)  // fd 0 = TUN
            pb.redirectErrorStream(true)                        // merge stderr into stdout
            coreProcess = pb.start()
        } finally {
            Os.dup2(savedStdin, 0)   // restore parent's stdin unconditionally
            Os.close(savedStdin)
        }

        val pid = try {
            val f = coreProcess!!.javaClass.getDeclaredField("pid")
            f.isAccessible = true
            f.getInt(coreProcess)
        } catch (_: Exception) { -1 }

        mihomoPid     = pid
        mihomoRunning = true
        LogEventBridge.info("mihomo", "Core started", mapOf("pid" to pid, "tun_fd" to tunFd))

        Thread {
            try {
                coreProcess?.inputStream?.bufferedReader()?.forEachLine { line ->
                    Log.d("MihomoCore", line)
                    LogEventBridge.debug("mihomo", line)
                }
                val exit = coreProcess?.exitValue() ?: -1
                mihomoRunning = false
                mihomoPid     = -1
                LogEventBridge.warn("mihomo", "Core exited", mapOf("exit_code" to exit))
            } catch (e: Exception) {
                LogEventBridge.error("mihomo", "Log reader error: ${e.message}")
            }
        }.start()
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
        LogEventBridge.info("vpn", "VPN revoked by system — another VPN app took over")
        stopVpn()
        super.onRevoke()
    }

    override fun onDestroy() {
        LogEventBridge.info("vpn", "Service onDestroy")
        stopVpn()
        super.onDestroy()
    }
}
