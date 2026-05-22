package com.clashforge.mobile.clashforge_mobile

import android.app.ActivityManager
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Process
import android.provider.Settings
import androidx.annotation.NonNull
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodChannel
import java.net.HttpURLConnection
import java.net.URL
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
        CronetEngine.Builder(applicationContext)
            .enableQuic(false)
            .enableHttp2(true)
            .enableBrotli(true)
            .build()
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
                        val timeoutMs = (call.argument<Int>("timeoutMs") ?: 15000)
                            .coerceIn(3000, 60000)
                        fetchUrlWithFallback(url, timeoutMs, result)
                    }
                    else -> result.notImplemented()
                }
            }
    }

    private fun fetchUrlWithFallback(
        url: String,
        timeoutMs: Int,
        result: MethodChannel.Result
    ) {
        fetchWithCronet(
            url = url,
            onSuccess = { code, body ->
                runOnUiThread { result.success(mapOf("status" to code, "body" to body)) }
            },
            onFailed = { cronetMessage ->
                cronetExecutor.execute {
                    try {
                        val (code, body) = fetchWithHttpURLConnection(url, timeoutMs)
                        runOnUiThread {
                            result.success(
                                mapOf(
                                    "status" to code,
                                    "body" to body
                                )
                            )
                        }
                    } catch (e: Exception) {
                        val fallbackMessage = e.message ?: "HttpURLConnection fetch failed"
                        runOnUiThread {
                            result.error(
                                "FETCH_ERROR",
                                "Cronet failed: $cronetMessage; HttpURLConnection failed: $fallbackMessage",
                                null
                            )
                        }
                    }
                }
            }
        )
    }

    private fun fetchWithCronet(
        url: String,
        onSuccess: (statusCode: Int, body: String) -> Unit,
        onFailed: (message: String) -> Unit
    ) {
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
                onSuccess(statusCode, body.toString())
            }

            override fun onFailed(
                request: UrlRequest,
                info: UrlResponseInfo?,
                error: CronetException
            ) {
                val infoText = info?.let { "url=${it.url}, status=${it.httpStatusCode}" } ?: "no_response_info"
                val message = "${error.javaClass.simpleName}: ${error.message ?: "Cronet fetch failed"} ($infoText)"
                onFailed(message)
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

    private fun fetchWithHttpURLConnection(url: String, timeoutMs: Int): Pair<Int, String> {
        val conn = (URL(url).openConnection() as HttpURLConnection).apply {
            requestMethod = "GET"
            connectTimeout = timeoutMs
            readTimeout = timeoutMs
            instanceFollowRedirects = true
            useCaches = false
            setRequestProperty(
                "User-Agent",
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
                    "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
            )
            setRequestProperty(
                "Accept",
                "text/html,application/xhtml+xml,application/xml;q=0.9," +
                    "image/avif,image/webp,image/apng,*/*;q=0.8," +
                    "application/signed-exchange;v=b3;q=0.7"
            )
            setRequestProperty("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
            setRequestProperty("Cache-Control", "no-cache")
        }

        try {
            val status = conn.responseCode
            val stream = if (status >= 400) conn.errorStream else conn.inputStream
            val body = stream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() } ?: ""
            return Pair(status, body)
        } finally {
            conn.disconnect()
        }
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

        val privateDnsMode = readGlobalSetting("private_dns_mode")
        val privateDnsSpecifier = readGlobalSetting("private_dns_specifier")

        return mapOf(
            "app_version"         to versionName,
            "build_number"        to versionCode,
            "vpn_running"         to ClashVpnService.vpnRunning,
            "mihomo_running"      to ClashVpnService.mihomoRunning,
            "mihomo_pid"          to ClashVpnService.mihomoPid,
            "memory_app_pss_mb"   to String.format("%.1f", pss).toDouble(),
            "memory_available_mb" to String.format("%.0f", memInfo.availMem / 1024.0 / 1024.0).toDouble(),
            "device_abi"          to (Build.SUPPORTED_ABIS.firstOrNull() ?: "unknown"),
            "private_dns_mode"    to privateDnsMode,
            "private_dns_specifier" to privateDnsSpecifier
        )
    }

    private fun readGlobalSetting(key: String): String {
        return try {
            Settings.Global.getString(contentResolver, key) ?: ""
        } catch (_: Exception) {
            ""
        }
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
