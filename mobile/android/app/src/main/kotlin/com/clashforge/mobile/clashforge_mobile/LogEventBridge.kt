package com.clashforge.mobile.clashforge_mobile

import android.os.Handler
import android.os.Looper
import io.flutter.plugin.common.EventChannel
import org.json.JSONObject
import java.util.concurrent.atomic.AtomicInteger

/**
 * Singleton bridge that lets any Android component (Service, etc.) push
 * structured log events to Flutter via EventChannel.
 *
 * Usage from Kotlin:
 *   LogEventBridge.info("vpn", "VPN interface established", mapOf("addr" to "172.19.0.1"))
 */
object LogEventBridge : EventChannel.StreamHandler {
    private var sink: EventChannel.EventSink? = null
    private val mainHandler = Handler(Looper.getMainLooper())

    // Backpressure: mihomo's stdout is forwarded line-by-line from a
    // background thread (ClashVpnService.startMihomoCore) with no rate limit
    // of its own. Under connection churn (Play Store retrying a stalled
    // download every 30-90s, each retry producing a burst of DNS/rule log
    // lines) that thread can produce lines faster than the main thread drains
    // posted Handler messages, so unbounded posting piles up JSONObject/
    // String allocations on the queue until the app OOMs — observed crashing
    // even at log-level: info, since info still emits a line per connection.
    private val pending = AtomicInteger(0)
    private const val MAX_PENDING = 500

    override fun onListen(arguments: Any?, events: EventChannel.EventSink) {
        sink = events
    }

    override fun onCancel(arguments: Any?) {
        sink = null
    }

    fun emit(level: String, component: String, message: String, fields: Map<String, Any> = emptyMap()) {
        NativeLogStore.append(level, component, message, fields)

        // Nobody listening (Flutter side hasn't opened the debug log stream) —
        // skip building the payload at all, not just the post.
        if (sink == null) return
        if (pending.incrementAndGet() > MAX_PENDING) {
            pending.decrementAndGet()
            return
        }
        val obj = JSONObject().apply {
            put("level", level)
            put("component", component)
            put("message", message)
            put("ts", System.currentTimeMillis())
            if (fields.isNotEmpty()) {
                val f = JSONObject()
                fields.forEach { (k, v) -> f.put(k, v) }
                put("fields", f)
            }
        }
        val payload = obj.toString()
        mainHandler.post {
            pending.decrementAndGet()
            sink?.success(payload)
        }
    }

    fun debug(component: String, message: String, fields: Map<String, Any> = emptyMap()) =
        emit("debug", component, message, fields)

    fun info(component: String, message: String, fields: Map<String, Any> = emptyMap()) =
        emit("info", component, message, fields)

    fun warn(component: String, message: String, fields: Map<String, Any> = emptyMap()) =
        emit("warn", component, message, fields)

    fun error(component: String, message: String, fields: Map<String, Any> = emptyMap()) =
        emit("error", component, message, fields)
}
