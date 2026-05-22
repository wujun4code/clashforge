package com.clashforge.mobile.clashforge_mobile

import android.app.ActivityManager
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Process
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.annotation.NonNull
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {
    private val VPN_CHANNEL = "com.clashforge.mobile/vpn"
    private val LOG_CHANNEL = "com.clashforge.mobile/logs"
    private val HTTP_CHANNEL = "com.clashforge.mobile/http"

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

        // HTTP channel: uses a hidden WebView (= Chrome TLS stack, identical JA3 fingerprint
        // to the system browser) to fetch subscription URLs that block non-browser TLS clients.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, HTTP_CHANNEL)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "fetchUrl" -> {
                        val url = call.argument<String>("url") ?: run {
                            result.error("INVALID_ARG", "url is required", null)
                            return@setMethodCallHandler
                        }
                        // WebView must be created and used on the main (UI) thread.
                        runOnUiThread { fetchWithWebView(url, result) }
                    }
                    else -> result.notImplemented()
                }
            }
    }

    // Loads `url` in a hidden WebView and returns the plain-text body via `result`.
    // Using WebView means we get Chrome's exact TLS/JA3 fingerprint, which passes
    // fingerprint-gating subscription servers that block HttpURLConnection and Dart http.
    private fun fetchWithWebView(url: String, result: MethodChannel.Result) {
        val wv = WebView(this)
        wv.settings.javaScriptEnabled = true
        wv.webViewClient = object : WebViewClient() {
            private var done = false

            override fun onPageFinished(view: WebView, pageUrl: String) {
                if (done) return
                done = true
                // document.documentElement.innerText captures raw text for plain-text
                // responses (JSON/YAML rendered as <pre> by Chrome).
                view.evaluateJavascript("document.documentElement.innerText") { raw ->
                    try {
                        // evaluateJavascript returns a JSON-encoded string; decode it.
                        val body = org.json.JSONArray("[$raw]").getString(0)
                        result.success(mapOf("status" to 200, "body" to body))
                    } catch (e: Exception) {
                        result.error("PARSE_ERROR", e.message ?: "parse failed", null)
                    }
                }
            }

            override fun onReceivedError(
                view: WebView, req: WebResourceRequest, err: WebResourceError
            ) {
                if (req.isForMainFrame && !done) {
                    done = true
                    result.error("FETCH_ERROR", err.description.toString(), null)
                }
            }

            @Suppress("DEPRECATION", "OverridingDeprecatedMember")
            override fun onReceivedError(
                view: WebView, code: Int, desc: String, failUrl: String
            ) {
                if (!done) {
                    done = true
                    result.error("FETCH_ERROR", desc, null)
                }
            }
        }
        wv.loadUrl(url)
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
