package com.clashforge.mobile.clashforge_mobile

import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
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
        const val ACTION_STOP = "com.clashforge.mobile.STOP"
        private const val TAG = "ClashVpnService"
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action
        if (ACTION_START == action) {
            startVpn()
        } else if (ACTION_STOP == action) {
            stopVpn()
        }
        return START_STICKY
    }

    @Synchronized
    private fun startVpn() {
        if (isRunning) {
            LogEventBridge.warn("vpn", "startVpn called but already running")
            return
        }
        isRunning = true
        LogEventBridge.info("vpn", "Starting VPN thread")
        vpnThread = Thread(this, "ClashVpnThread").apply { start() }
    }

    @Synchronized
    private fun stopVpn() {
        if (!isRunning) {
            LogEventBridge.warn("vpn", "stopVpn called but not running")
            return
        }
        isRunning = false
        LogEventBridge.info("vpn", "Stopping VPN")

        coreProcess?.destroy()
        coreProcess = null
        LogEventBridge.debug("vpn", "Mihomo core process destroyed")

        try {
            vpnInterface?.close()
            LogEventBridge.debug("vpn", "VPN interface closed")
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

            LogEventBridge.info("vpn", "Building VPN interface", mapOf(
                "addr" to "172.19.0.1/30",
                "route" to "0.0.0.0/0",
                "dns" to "172.19.0.2"
            ))

            val builder = Builder()
                .addAddress("172.19.0.1", 30)
                .addRoute("0.0.0.0", 0)
                .addDnsServer("172.19.0.2")
                .setSession("ClashForge")
                .setBlocking(false)

            vpnInterface = builder.establish()

            if (vpnInterface == null) {
                LogEventBridge.error("vpn", "builder.establish() returned null — VPN permission may not be granted")
                return
            }

            LogEventBridge.info("vpn", "VPN interface established", mapOf(
                "fd" to (vpnInterface?.fd ?: -1)
            ))

            startMihomoCore()

            LogEventBridge.info("vpn", "VPN loop running")
            while (isRunning) {
                Thread.sleep(1000)
            }
        } catch (e: Exception) {
            Log.e(TAG, "VPN service execution error", e)
            LogEventBridge.error("vpn", "VPN run() exception: ${e.message}", mapOf(
                "type" to e.javaClass.simpleName
            ))
        } finally {
            stopVpn()
        }
    }

    // Extract mihomo binary and geodata from Flutter assets to filesDir.
    // Re-extracts whenever the app versionCode changes (i.e. on update).
    private fun extractAssetsIfNeeded() {
        val markerFile = File(filesDir, ".asset_version")
        val currentVersion = packageManager
            .getPackageInfo(packageName, 0)
            .let { if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) it.longVersionCode.toString() else @Suppress("DEPRECATION") it.versionCode.toString() }

        if (markerFile.exists() && markerFile.readText().trim() == currentVersion) {
            LogEventBridge.debug("assets", "Assets up-to-date, skipping extraction", mapOf("version" to currentVersion))
            return
        }

        LogEventBridge.info("assets", "Extracting assets", mapOf("version" to currentVersion))

        val abi = Build.SUPPORTED_ABIS.firstOrNull() ?: "arm64-v8a"
        LogEventBridge.debug("assets", "Device ABI", mapOf("abi" to abi))

        extractAsset("flutter_assets/assets/mihomo/$abi", File(filesDir, "mihomo"), executable = true)
        extractAsset("flutter_assets/assets/geodata/country.mmdb", File(filesDir, "country.mmdb"))
        extractAsset("flutter_assets/assets/geodata/geosite.dat", File(filesDir, "geosite.dat"))

        markerFile.writeText(currentVersion)
        LogEventBridge.info("assets", "Asset extraction complete")
    }

    private fun extractAsset(assetPath: String, dest: File, executable: Boolean = false) {
        try {
            assets.open(assetPath).use { input ->
                FileOutputStream(dest).use { output -> input.copyTo(output) }
            }
            if (executable) dest.setExecutable(true)
            LogEventBridge.debug("assets", "Extracted ${dest.name}", mapOf(
                "size" to dest.length(),
                "path" to dest.absolutePath
            ))
        } catch (e: Exception) {
            Log.e(TAG, "Failed to extract asset $assetPath", e)
            LogEventBridge.error("assets", "Extract failed: ${dest.name}", mapOf(
                "asset" to assetPath,
                "error" to (e.message ?: "unknown")
            ))
        }
    }

    private fun startMihomoCore() {
        try {
            val appDir = filesDir.absolutePath
            val coreBin = File(appDir, "mihomo")

            if (!coreBin.exists()) {
                LogEventBridge.warn("mihomo", "Binary not found — skipping core start", mapOf(
                    "path" to coreBin.absolutePath
                ))
                Log.w(TAG, "mihomo core binary does not exist yet at: ${coreBin.absolutePath}")
                return
            }

            coreBin.setExecutable(true)
            LogEventBridge.debug("mihomo", "Set executable: ${coreBin.absolutePath}")

            val configFile = File(appDir, "config.yaml")
            LogEventBridge.info("mihomo", "Starting core process", mapOf(
                "bin" to coreBin.absolutePath,
                "config" to configFile.absolutePath,
                "config_exists" to configFile.exists()
            ))

            val pb = ProcessBuilder(coreBin.absolutePath, "-d", appDir, "-f", configFile.absolutePath)
            pb.redirectErrorStream(true)
            coreProcess = pb.start()

            val pid = try {
                val pidField = coreProcess!!.javaClass.getDeclaredField("pid")
                pidField.isAccessible = true
                pidField.getInt(coreProcess)
            } catch (_: Exception) { -1 }

            LogEventBridge.info("mihomo", "Core process started", mapOf("pid" to pid))

            Thread {
                try {
                    coreProcess?.inputStream?.bufferedReader()?.forEachLine { line ->
                        Log.d("MihomoCore", line)
                        LogEventBridge.debug("mihomo", line)
                    }
                    val exitCode = coreProcess?.exitValue() ?: -1
                    LogEventBridge.warn("mihomo", "Core process exited", mapOf("exit_code" to exitCode))
                } catch (e: Exception) {
                    LogEventBridge.error("mihomo", "Core log reader error: ${e.message}")
                }
            }.start()

        } catch (e: Exception) {
            Log.e(TAG, "Failed to start mihomo core", e)
            LogEventBridge.error("mihomo", "Failed to start core: ${e.message}", mapOf(
                "type" to e.javaClass.simpleName
            ))
        }
    }

    override fun onDestroy() {
        LogEventBridge.info("vpn", "Service onDestroy")
        stopVpn()
        super.onDestroy()
    }
}
