package com.clashforge.mobile.clashforge_mobile

import android.app.ActivityManager
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Process
import androidx.annotation.NonNull
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {
    private val VPN_CHANNEL = "com.clashforge.mobile/vpn"
    private val LOG_CHANNEL = "com.clashforge.mobile/logs"

    override fun configureFlutterEngine(@NonNull flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)

        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, VPN_CHANNEL)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "startVpn" -> {
                        val intent = VpnService.prepare(this)
                        if (intent != null) {
                            startActivityForResult(intent, 0)
                            result.success("permission_needed")
                        } else {
                            startVpnService()
                            result.success("started")
                        }
                    }
                    "stopVpn" -> {
                        stopVpnService()
                        result.success("stopped")
                    }
                    "getFilesDir" -> {
                        result.success(filesDir.absolutePath)
                    }
                    "writeConfig" -> {
                        val yaml = call.argument<String>("yaml") ?: ""
                        try {
                            File(filesDir, "config.yaml").writeText(yaml)
                            result.success("written")
                        } catch (e: Exception) {
                            result.error("WRITE_FAILED", e.message, null)
                        }
                    }
                    "getSystemInfo" -> {
                        result.success(buildSystemInfo())
                    }
                    else -> result.notImplemented()
                }
            }

        EventChannel(flutterEngine.dartExecutor.binaryMessenger, LOG_CHANNEL)
            .setStreamHandler(LogEventBridge)
    }

    private fun buildSystemInfo(): Map<String, Any> {
        val pm = packageManager.getPackageInfo(packageName, 0)
        val versionName = pm.versionName ?: "unknown"
        val versionCode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P)
            pm.longVersionCode else @Suppress("DEPRECATION") pm.versionCode.toLong()

        val am = getSystemService(ACTIVITY_SERVICE) as ActivityManager
        val memInfo = ActivityManager.MemoryInfo()
        am.getMemoryInfo(memInfo)

        val pss = try {
            am.getProcessMemoryInfo(intArrayOf(Process.myPid()))[0].totalPss / 1024.0
        } catch (_: Exception) { 0.0 }

        return mapOf(
            "app_version"         to versionName,
            "build_number"        to versionCode,
            "vpn_running"         to ClashVpnService.vpnRunning,
            "mihomo_running"      to ClashVpnService.mihomoRunning,
            "mihomo_pid"          to ClashVpnService.mihomoPid,
            "memory_app_pss_mb"   to String.format("%.1f", pss).toDouble(),
            "memory_available_mb" to String.format("%.0f", memInfo.availMem / 1024.0 / 1024.0).toDouble(),
            "device_abi"          to (Build.SUPPORTED_ABIS.firstOrNull() ?: "unknown")
        )
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == 0 && resultCode == RESULT_OK) startVpnService()
    }

    private fun startVpnService() {
        startService(Intent(this, ClashVpnService::class.java).apply {
            action = ClashVpnService.ACTION_START
        })
    }

    private fun stopVpnService() {
        startService(Intent(this, ClashVpnService::class.java).apply {
            action = ClashVpnService.ACTION_STOP
        })
    }
}
