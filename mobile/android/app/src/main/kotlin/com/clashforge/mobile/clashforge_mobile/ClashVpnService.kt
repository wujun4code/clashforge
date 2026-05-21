package com.clashforge.mobile.clashforge_mobile

import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.system.Os
import android.util.Log
import java.io.File
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

    // Holds the Os.dup() copy of the TUN fd so stopVpn() can close it.
    // Without this, Android keeps the VPN tunnel alive even after vpnInterface.close().
    private var dupTunFd: java.io.FileDescriptor? = null

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

        // Close the dup'd TUN fd first — this is the one the service process kept open
        // after handing the fd to mihomo. Without closing it the OS keeps the tunnel alive.
        try {
            dupTunFd?.let { android.system.Os.close(it) }
        } catch (e: Exception) {
            LogEventBridge.warn("vpn", "Error closing dup'd TUN fd: ${e.message}")
        }
        dupTunFd = null

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
                .setSession("ClashForge")
                .setBlocking(false)

            vpnInterface = builder.establish() ?: run {
                LogEventBridge.error("vpn", "builder.establish() returned null")
                return
            }

            val tunFd = vpnInterface!!.fd
            LogEventBridge.info("vpn", "VPN interface established", mapOf("fd" to tunFd))

            // Os.dup() creates a copy of the fd WITHOUT FD_CLOEXEC (by POSIX spec),
            // so mihomo's child process can inherit it via /proc/self/fd/N.
            // We save the FileDescriptor in dupTunFd so stopVpn() can close it —
            // if left open, the VPN tunnel stays active even after vpnInterface.close().
            var inheritableFd = tunFd
            try {
                val dupFileDes = Os.dup(vpnInterface!!.fileDescriptor)
                dupTunFd = dupFileDes
                val fdField = java.io.FileDescriptor::class.java.getDeclaredField("descriptor")
                fdField.isAccessible = true
                inheritableFd = fdField.getInt(dupFileDes)
                LogEventBridge.debug("vpn", "Duplicated TUN fd (no CLOEXEC)",
                    mapOf("orig" to tunFd, "dup" to inheritableFd))
            } catch (e: Exception) {
                LogEventBridge.warn("vpn", "Could not dup TUN fd, using original: ${e.message}")
            }

            // Patch config.yaml with the TUN section now that an inheritable fd is known
            patchConfigWithTun(inheritableFd)

            startMihomoCore()

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
  device: "/proc/self/fd/$tunFd"
  auto-route: false
  auto-detect-interface: false
  mtu: 1500
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
        val tunStanza = """

tun:
  enable: true
  stack: gvisor
  device: "/proc/self/fd/$tunFd"
  auto-route: false
  auto-detect-interface: false
  mtu: 1500
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
            "fd"     to tunFd,
            "device" to "/proc/self/fd/$tunFd"
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

    private fun startMihomoCore() {
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
            "config_exists" to configFile.exists(),
            "config_size"   to configFile.length()
        ))

        val pb = ProcessBuilder(coreBin.absolutePath, "-d", appDir, "-f", configFile.absolutePath)
        pb.redirectErrorStream(true)   // capture stderr+stdout via inputStream → Log.d("MihomoCore")
        coreProcess = pb.start()

        val pid = try {
            val f = coreProcess!!.javaClass.getDeclaredField("pid")
            f.isAccessible = true
            f.getInt(coreProcess)
        } catch (_: Exception) { -1 }

        mihomoPid     = pid
        mihomoRunning = true
        LogEventBridge.info("mihomo", "Core started", mapOf("pid" to pid))

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
