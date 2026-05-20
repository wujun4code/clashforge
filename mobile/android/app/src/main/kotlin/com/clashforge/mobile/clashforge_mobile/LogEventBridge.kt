package com.clashforge.mobile.clashforge_mobile

import android.os.Handler
import android.os.Looper
import io.flutter.plugin.common.EventChannel
import org.json.JSONObject

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

    override fun onListen(arguments: Any?, events: EventChannel.EventSink) {
        sink = events
    }

    override fun onCancel(arguments: Any?) {
        sink = null
    }

    fun emit(level: String, component: String, message: String, fields: Map<String, Any> = emptyMap()) {
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
        mainHandler.post { sink?.success(payload) }
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
