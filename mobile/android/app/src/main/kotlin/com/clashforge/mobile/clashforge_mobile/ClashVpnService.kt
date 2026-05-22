package com.clashforge.mobile.clashforge_mobile

import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.system.Os
import android.util.Log
import java.io.File
import java.io.FileDescriptor
import java.io.FileOutputStream

class ClashVpnService : VpnService(), Runnable {
    private var vpnThread: Thread? = null
    private var vpnInterface: ParcelFileDescriptor? = null
    private var isRunning = false
    private var coreProcess: Process? = null

    companion object {
        const val ACTION_START = "com.clashforge.mobile.START"
        const val ACTION_STOP  = "com.clashforge.mobile.STOP"
        private const val TAG  = "ClashVpnService"

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

            LogEventBridge.info("vpn", "Building VPN interface",
                mapOf("addr" to "172.19.0.1/30", "route" to "0.0.0.0/0", "dns" to "172.19.0.1"))

            val builder = Builder()
                .addAddress("172.19.0.1", 30)
                .addRoute("0.0.0.0", 0)
                .addDnsServer("172.19.0.1")   // mihomo listens here after patching
                .setMtu(1500)                 // explicit MTU so sing-tun doesn't need to set it
                .setSession("ClashForge")
                .setBlocking(false)

            vpnInterface = builder.establish() ?: run {
                LogEventBridge.error("vpn", "builder.establish() returned null")
                return
            }

            val tunFd = vpnInterface!!.fd
            LogEventBridge.info("vpn", "VPN interface established", mapOf("fd" to tunFd))

            // Android closes ALL fds >= 3 in the child before exec (closeDescriptors).
            // Only fds 0/1/2 survive.  We dup the TUN fd onto fd 0 (stdin) in the parent
            // before forking, tell ProcessBuilder to INHERIT stdin, then restore fd 0.
            // POSIX dup2 clears FD_CLOEXEC on the new fd, so fd 0 survives exec.
            // sing-tun is built from source with the sentinel changed from 0 to -1, so
            // file-descriptor: 0 means "use fd 0" and mihomo's stdout (fd 1) stays
            // connected to our log pipe — no interference with TUN reads/writes.
            patchConfigWithTun(0)

            startMihomoCore(tunFd)

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

    // Returns true when running inside a CI emulator that has forwarded Tor SOCKS5 via
    // "adb reverse tcp:9050 tcp:9050".  Uses a short timeout so it doesn't stall production.
    private fun isCiTorAvailable(): Boolean = try {
        java.net.Socket().use {
            it.connect(java.net.InetSocketAddress("127.0.0.1", 9050), 300)
        }
        true
    } catch (_: Exception) { false }

    // In CI, subscription proxy nodes are blocked from GitHub Actions (Azure) IPs.
    // When Tor is detected on 127.0.0.1:9050 (forwarded from the CI host via adb reverse),
    // we replace the entire config with a minimal one that uses Tor as the sole proxy.
    // This guarantees a different exit IP without depending on external proxy reachability.
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

    private fun patchConfigWithTun(tunFd: Int) {
        val configFile = File(filesDir, "config.yaml")

        if (isCiTorAvailable()) {
            LogEventBridge.info("vpn", "CI mode: Tor SOCKS5 detected on :9050 — writing CI config", mapOf("tunFd" to tunFd))
            configFile.writeText(buildCiConfig(tunFd))
            return
        }

        if (!configFile.exists()) {
            LogEventBridge.warn("vpn", "config.yaml not found — skipping TUN patch")
            return
        }
        // Append TUN + sniffer together.
        // sniffer enables QUIC/TLS/HTTP domain identification so mihomo can apply
        // domain-based rules to QUIC (HTTP/3) connections via the gvisor TUN stack.
        // Without sniffer, QUIC to HTTP-proxy nodes causes a silent timeout before
        // the app falls back to TCP; with sniffer mihomo identifies the domain and
        // can fast-fail unsupported UDP sessions.
        // If the subscription config already has a sniffer block this override is
        // intentional — our config is always more complete for VPN usage.
        // file-descriptor: 0 = mihomo's fd 0 (stdin), which startMihomoCore temporarily
        // replaces with the TUN fd via Os.dup2 before forking.  sing-tun is patched to
        // treat FileDescriptor<0 (not 0) as "not set", so 0 means use fd 0 directly.
        // sing-tun.New() skips configure() entirely when its sentinel check is false.
        val tunStanza = """

tun:
  enable: true
  stack: gvisor
  file-descriptor: $tunFd
  auto-route: false
  auto-detect-interface: false
  dns-hijack:
    - "any:53"

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
"""
        configFile.appendText(tunStanza)
        LogEventBridge.info("vpn", "Patched config.yaml with TUN fd", mapOf(
            "file-descriptor" to tunFd,
            "stack"           to "gvisor",
            "note"            to "stdin(fd0)=TUN"
        ))
    }

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

    private fun startMihomoCore(tunFd: Int) {
        val appDir     = filesDir.absolutePath
        // nativeLibraryDir is on an executable partition; filesDir is mounted noexec on API 29+
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
            "tun_fd"        to tunFd
        ))

        val tunFdObj = vpnInterface?.fileDescriptor ?: run {
            LogEventBridge.error("mihomo", "vpnInterface null before spawn")
            return
        }

        // Android closes ALL fds >= 3 in the child before exec (closeDescriptors).
        // The only fds that survive into mihomo are 0 (stdin), 1 (stdout), 2 (stderr).
        //
        // Strategy: dup the TUN fd onto fd 0 (stdin) in THIS process, fork with
        // INHERIT for stdin so the child gets fd 0 = TUN, then restore fd 0 here.
        // POSIX dup2() always clears FD_CLOEXEC on the new fd, so fd 0 survives exec.
        // sing-tun is patched (FileDescriptor<0 = "not set") so file-descriptor: 0
        // means "use fd 0".  mihomo's stdout (fd 1) and stderr (fd 2) stay connected
        // to normal pipes — no write errors, no interference with TUN reads.
        val savedStdin: FileDescriptor = Os.dup(FileDescriptor.`in`)
        try {
            Os.dup2(tunFdObj, 0)   // fd 0 in this process = TUN fd

            val pb = ProcessBuilder(coreBin.absolutePath, "-d", appDir, "-f", configFile.absolutePath)
            pb.redirectInput(ProcessBuilder.Redirect.INHERIT)  // fd 0 = TUN (no dup2 by JVM)
            pb.redirectErrorStream(true)                       // merge stderr into stdout pipe
            coreProcess = pb.start()
        } finally {
            Os.dup2(savedStdin, 0)   // restore parent's stdin regardless of outcome
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

    override fun onDestroy() {
        LogEventBridge.info("vpn", "Service onDestroy")
        stopVpn()
        super.onDestroy()
    }
}
