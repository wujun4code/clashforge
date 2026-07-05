package com.clashforge.mobile.clashforge_mobile

import android.content.Context
import org.json.JSONObject
import java.io.File

object NativeLogStore {
    private const val FILE_NAME = "native_runtime.log"
    private const val MAX_BYTES = 768 * 1024
    private val lock = Any()

    @Volatile private var appContext: Context? = null

    fun init(context: Context) {
        appContext = context.applicationContext
    }

    fun append(
        level: String,
        component: String,
        message: String,
        fields: Map<String, Any> = emptyMap(),
    ) {
        val context = appContext ?: return
        try {
            val record = JSONObject().apply {
                put("ts", System.currentTimeMillis())
                put("level", level)
                put("component", component)
                put("message", message)
                if (fields.isNotEmpty()) {
                    val fieldObj = JSONObject()
                    fields.forEach { (key, value) -> fieldObj.put(key, value) }
                    put("fields", fieldObj)
                }
            }
            val line = record.toString() + "\n"
            synchronized(lock) {
                val file = File(context.filesDir, FILE_NAME)
                trimIfNeeded(file, line.toByteArray(Charsets.UTF_8).size)
                file.appendText(line, Charsets.UTF_8)
            }
        } catch (_: Exception) {
            // Logging must never affect the VPN datapath.
        }
    }

    fun read(): String {
        val context = appContext ?: return ""
        return try {
            val file = File(context.filesDir, FILE_NAME)
            if (!file.exists()) "" else synchronized(lock) {
                file.readText(Charsets.UTF_8)
            }
        } catch (_: Exception) {
            ""
        }
    }

    fun clear() {
        val context = appContext ?: return
        try {
            synchronized(lock) {
                File(context.filesDir, FILE_NAME).delete()
            }
        } catch (_: Exception) {
            // Best effort only.
        }
    }

    private fun trimIfNeeded(file: File, incomingBytes: Int) {
        if (!file.exists()) return
        if (file.length() + incomingBytes <= MAX_BYTES) return

        val bytes = file.readBytes()
        val keepBytes = MAX_BYTES * 3 / 4
        val start = (bytes.size - keepBytes).coerceAtLeast(0)
        val tail = bytes.copyOfRange(start, bytes.size)
        val firstNewline = tail.indexOf('\n'.code.toByte())
        val cleanTail = if (firstNewline in 0 until tail.lastIndex) {
            tail.copyOfRange(firstNewline + 1, tail.size)
        } else {
            tail
        }
        file.writeBytes(cleanTail)
    }
}
