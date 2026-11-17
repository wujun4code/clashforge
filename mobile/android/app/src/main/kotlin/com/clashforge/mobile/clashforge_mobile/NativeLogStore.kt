package com.clashforge.mobile.clashforge_mobile

import android.content.Context
import org.json.JSONObject
import java.io.BufferedWriter
import java.io.File
import java.io.FileOutputStream
import java.io.OutputStreamWriter

object NativeLogStore {
    private const val FILE_NAME = "native_runtime.log"
    private const val ARCHIVE_FILE_NAME = "native_runtime.1.log"
    private const val MAX_BYTES = 512 * 1024
    private const val FLUSH_EVERY = 16
    private val lock = Any()

    @Volatile private var appContext: Context? = null
    private var writer: BufferedWriter? = null
    private var writerFile: File? = null
    private var pendingWrites = 0

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
                rotateIfNeeded(file, line.toByteArray(Charsets.UTF_8).size)
                val output = openWriter(file)
                output.write(line)
                pendingWrites++
                if (pendingWrites >= FLUSH_EVERY || level == "warn" || level == "error") {
                    output.flush()
                    pendingWrites = 0
                }
            }
        } catch (_: Exception) {
            // Logging must never affect the VPN datapath.
        }
    }

    fun read(): String {
        val context = appContext ?: return ""
        return try {
            synchronized(lock) {
                flushWriter()
                val archive = File(context.filesDir, ARCHIVE_FILE_NAME)
                val current = File(context.filesDir, FILE_NAME)
                buildString {
                    if (archive.exists()) append(archive.readText(Charsets.UTF_8))
                    if (current.exists()) append(current.readText(Charsets.UTF_8))
                }
            }
        } catch (_: Exception) {
            ""
        }
    }

    fun clear() {
        val context = appContext ?: return
        try {
            synchronized(lock) {
                closeWriter()
                File(context.filesDir, FILE_NAME).delete()
                File(context.filesDir, ARCHIVE_FILE_NAME).delete()
            }
        } catch (_: Exception) {
            // Best effort only.
        }
    }

    private fun rotateIfNeeded(file: File, incomingBytes: Int) {
        if (!file.exists() || file.length() + incomingBytes <= MAX_BYTES) return
        closeWriter()
        val archive = File(file.parentFile, ARCHIVE_FILE_NAME)
        if (archive.exists()) archive.delete()
        if (!file.renameTo(archive)) {
            // Rotation is best-effort. Truncating is still bounded and avoids the
            // previous read/copy/rewrite of the complete log on the VPN datapath.
            file.writeText("", Charsets.UTF_8)
        }
    }

    private fun openWriter(file: File): BufferedWriter {
        if (writer != null && writerFile == file) return writer!!
        closeWriter()
        return BufferedWriter(
            OutputStreamWriter(FileOutputStream(file, true), Charsets.UTF_8),
            16 * 1024,
        ).also {
            writer = it
            writerFile = file
        }
    }

    private fun flushWriter() {
        writer?.flush()
        pendingWrites = 0
    }

    private fun closeWriter() {
        try {
            writer?.flush()
            writer?.close()
        } finally {
            writer = null
            writerFile = null
            pendingWrites = 0
        }
    }
}
