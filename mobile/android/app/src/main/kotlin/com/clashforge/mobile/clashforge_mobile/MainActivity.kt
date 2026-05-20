package com.clashforge.mobile.clashforge_mobile

import android.content.Intent
import android.net.VpnService
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

        // VPN control channel (Flutter → Native)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, VPN_CHANNEL).setMethodCallHandler { call, result ->
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
                else -> result.notImplemented()
            }
        }

        // Log event channel (Native → Flutter)
        EventChannel(flutterEngine.dartExecutor.binaryMessenger, LOG_CHANNEL)
            .setStreamHandler(LogEventBridge)
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == 0 && resultCode == RESULT_OK) {
            startVpnService()
        }
    }

    private fun startVpnService() {
        val intent = Intent(this, ClashVpnService::class.java).apply {
            action = ClashVpnService.ACTION_START
        }
        startService(intent)
    }

    private fun stopVpnService() {
        val intent = Intent(this, ClashVpnService::class.java).apply {
            action = ClashVpnService.ACTION_STOP
        }
        startService(intent)
    }
}
