package com.clashforge.mobile.clashforge_mobile

import org.json.JSONObject

/**
 * JNI bridge to libcfgen.so — the Go-based config-generation library.
 *
 * Two-step flow (called from ClashVpnService):
 *
 *   Step 1 — before VPN interface is up:
 *     val probe = ConfigGen.probeAndPatchDNS(configPath)
 *     // Probes configured UDP nameservers against sampled proxy hostnames.
 *     // If GFW DNS hijacking is detected, rewrites dns.nameserver/fallback
 *     // in config.yaml to DoH-only entries.  Probe sockets travel the
 *     // physical network (not the tunnel) because the VPN isn't up yet.
 *
 *   Step 2 — after builder.establish() returns the TUN fd:
 *     val ok = ConfigGen.generateConfig(configPath, tunFd = 0, geoDataDir)
 *     // Reads the (possibly probe-patched) subscription YAML, rewrites the
 *     // dns/tun/sniffer sections for Android TUN mode, and writes the final
 *     // config back to configPath.
 */
object ConfigGen {
    init {
        System.loadLibrary("cfgen")
    }

    // ── Raw JNI exports (names must match Go //export symbols exactly) ─────

    @JvmStatic
    private external fun nativeProbeAndPatchDNS(configPath: String): String

    @JvmStatic
    private external fun nativeGenerateConfig(
        configPath: String,
        tunFd: Int,
        geoDataDir: String,
        dnsMode: String,
    ): String

    // ── Typed wrappers ────────────────────────────────────────────────────

    data class ProbeResult(
        val summary: String,
        val wasPatched: Boolean,
    )

    /**
     * Probes upstream DNS and patches config.yaml if hijacking is detected.
     * Must be called BEFORE [generateConfig] and BEFORE the VPN interface
     * is established.
     */
    fun probeAndPatchDNS(configPath: String): ProbeResult {
        val json = try {
            JSONObject(nativeProbeAndPatchDNS(configPath))
        } catch (e: Exception) {
            LogEventBridge.error("cfgen", "probeAndPatchDNS parse error: ${e.message}")
            return ProbeResult(summary = "parse error: ${e.message}", wasPatched = false)
        }
        return ProbeResult(
            summary = json.optString("summary"),
            wasPatched = json.optBoolean("was_patched", false),
        )
    }

    /**
     * Generates the final Android-ready config.yaml.
     *
     * @param configPath path to config.yaml (read and overwritten in-place)
     * @param tunFd      TUN file-descriptor number; always pass 0 here because
     *                   [ClashVpnService.startMihomoCore] dup2s the real fd
     *                   onto stdin (fd 0) before forking mihomo.
     * @param geoDataDir directory containing geosite.dat / country.mmdb
     * @param dnsMode    "fake-ip" (default) or "redir-host"; see cfgen/generator.go
     * @return true on success
     */
    fun generateConfig(configPath: String, tunFd: Int, geoDataDir: String, dnsMode: String = "fake-ip"): Boolean {
        val json = try {
            JSONObject(nativeGenerateConfig(configPath, tunFd, geoDataDir, dnsMode))
        } catch (e: Exception) {
            LogEventBridge.error("cfgen", "generateConfig parse error: ${e.message}")
            return false
        }
        val err = json.optString("error")
        if (err.isNotEmpty()) {
            LogEventBridge.error("cfgen", "generateConfig failed: $err")
        }
        return json.optBoolean("ok", false)
    }
}
