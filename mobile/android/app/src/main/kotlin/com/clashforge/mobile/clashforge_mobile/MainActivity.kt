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
import java.nio.ByteBuffer
import java.util.concurrent.Executor
import java.util.concurrent.Executors
import org.chromium.net.CronetEngine
import org.chromium.net.CronetException
import org.chromium.net.UrlRequest
import org.chromium.net.UrlResponseInfo

class MainActivity : FlutterActivity() {
    private val VPN_CHANNEL = "com.clashforge.mobile/vpn"
    private val LOG_CHANNEL = "com.clashforge.mobile/logs"
    private val HTTP_CHANNEL = "com.clashforge.mobile/http"

    // Cronet uses Chrome's actual TLS stack — same JA3 fingerprint as desktop Chrome.
    // Subscription servers that reject OkHttp (Android TLS, different JA3) accept Cronet.
    private val cronetEngine: CronetEngine by lazy {
        CronetEngine.Builder(applicationContext).build()
    }
    private val cronetExecutor: Executor = Executors.newSingleThreadExecutor()

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

        // HTTP channel: Cronet uses Chrome's BoringSSL TLS stack with identical cipher suite
        // ordering and extension set, so the JA3 fingerprint matches desktop Chrome.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, HTTP_CHANNEL)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "fetchUrl" -> {
                        val url = call.argument<String>("url") ?: run {
                            result.error("INVALID_ARG", "url is required", null)
                            return@setMethodCallHandler
                        }
                        fetchWithCronet(url, result)
                    }
                    else -> result.notImplemented()
                }
            }
    }

    private fun fetchWithCronet(url: String, result: MethodChannel.Result) {
        val callback = object : UrlRequest.Callback() {
            private var statusCode = 0
            private val body = StringBuilder()
            private val buffer = ByteBuffer.allocateDirect(32 * 1024)

            override fun onRedirectReceived(
                request: UrlRequest,
                info: UrlResponseInfo,
                newLocationUrl: String
            ) {
                request.followRedirect()
            }

            override fun onResponseStarted(request: UrlRequest, info: UrlResponseInfo) {
                statusCode = info.httpStatusCode
                buffer.clear()
                request.read(buffer)
            }

            override fun onReadCompleted(
                request: UrlRequest,
                info: UrlResponseInfo,
                byteBuffer: ByteBuffer
            ) {
                byteBuffer.flip()
                val bytes = ByteArray(byteBuffer.remaining())
                byteBuffer.get(bytes)
                body.append(String(bytes, Charsets.UTF_8))
                byteBuffer.clear()
                request.read(byteBuffer)
            }

            override fun onSucceeded(request: UrlRequest, info: UrlResponseInfo) {
                val code = statusCode
                val responseBody = body.toString()
                runOnUiThread {
                    result.success(mapOf("status" to code, "body" to responseBody))
                }
            }

            override fun onFailed(
                request: UrlRequest,
                info: UrlResponseInfo?,
                error: CronetException
            ) {
                runOnUiThread {
                    result.error("FETCH_ERROR", error.message ?: "Cronet fetch failed", null)
                }
            }
        }

        cronetEngine.newUrlRequestBuilder(url, callback, cronetExecutor)
            .addHeader("User-Agent",
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
                "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
            .addHeader("Accept",
                "text/html,application/xhtml+xml,application/xml;q=0.9," +
                "image/avif,image/webp,image/apng,*/*;q=0.8," +
                "application/signed-exchange;v=b3;q=0.7")
            .addHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
            .addHeader("Cache-Control", "no-cache")
            .build()
            .start()
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

    override fun onDestroy() {
        super.onDestroy()
        cronetEngine.shutdown()
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
