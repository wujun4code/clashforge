package com.clashforge.mobile.clashforge_mobile

import android.content.Intent
import android.net.VpnService
import android.os.ParcelFileDescriptor
import android.util.Log
import java.io.File
import java.io.FileOutputStream
import java.io.IOException

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
        if (isRunning) return
        isRunning = true
        vpnThread = Thread(this, "ClashVpnThread").apply { start() }
    }

    @Synchronized
    private fun stopVpn() {
        if (!isRunning) return
        isRunning = false
        
        // Stop Mihomo process
        coreProcess?.destroy()
        coreProcess = null

        // Close Tun Interface
        try {
            vpnInterface?.close()
        } catch (e: Exception) {
            Log.e(TAG, "Error closing VPN interface", e)
        }
        vpnInterface = null

        stopSelf()
    }

    override fun run() {
        try {
            // 1. Establish VPN interface
            val builder = Builder()
                .addAddress("172.19.0.1", 30)
                .addRoute("0.0.0.0", 0)
                .addDnsServer("172.19.0.2") // Route everything into tun
                .setSession("ClashForge")
                .setBlocking(false)
            
            vpnInterface = builder.establish()
            Log.i(TAG, "VPN Interface established successfully")

            // 2. Extract tun file descriptor to pass or let mihomo listen via tun-device / auto-route
            // In a simple production scenario, mihomo runs on host or takes the tun fd directly.
            // For MVP setup, we can start mihomo in a background process with auto-detect tun/tun-device or fake-tun.
            // Here we run mihomo config
            startMihomoCore()

            while (isRunning) {
                Thread.sleep(1000)
            }
        } catch (e: Exception) {
            Log.e(TAG, "VPN service execution error", e)
        } finally {
            stopVpn()
        }
    }

    private fun startMihomoCore() {
        try {
            val appDir = filesDir.absolutePath
            val coreBin = File(appDir, "mihomo")
            if (!coreBin.exists()) {
                Log.w(TAG, "mihomo core binary does not exist yet at: ${coreBin.absolutePath}")
                return
            }
            
            // Set executable permissions
            coreBin.setExecutable(true)

            val builder = ProcessBuilder(
                coreBin.absolutePath,
                "-d", appDir,
                "-f", File(appDir, "config.yaml").absolutePath
            )
            builder.redirectErrorStream(true)
            coreProcess = builder.start()
            
            Thread {
                try {
                    val reader = coreProcess?.inputStream?.bufferedReader()
                    reader?.forEachLine { line ->
                        Log.d("MihomoCore", line)
                    }
                } catch (e: Exception) {
                    Log.e(TAG, "Error reading core logs", e)
                }
            }.start()

            Log.i(TAG, "Mihomo Core process started")
        } catch (e: Exception) {
            Log.e(TAG, "Failed to start mihomo core", e)
        }
    }

    override fun onDestroy() {
        stopVpn()
        super.onDestroy()
    }
}
