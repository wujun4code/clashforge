package com.clashforge.mobile.clashforge_mobile

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ComponentName
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.service.quicksettings.TileService
import android.system.Os
import android.util.Log
import androidx.core.app.NotificationCompat
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.FileDescriptor
import java.io.FileOutputStream
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.HttpURLConnection
import java.net.InetAddress
import java.net.URL
import java.util.Locale
import java.util.concurrent.ThreadLocalRandom

class ClashVpnService : VpnService(), Runnable {
    private var vpnThread: Thread? = null
    private var vpnInterface: ParcelFileDescriptor? = null
    private var isRunning = false
    private var coreProcess: Process? = null

    companion object {
        const val ACTION_START = "com.clashforge.mobile.START"
        const val ACTION_STOP  = "com.clashforge.mobile.STOP"
        const val ACTION_VPN_STATE_CHANGED = "com.clashforge.mobile.VPN_STATE_CHANGED"
        const val EXTRA_VPN_RUNNING = "vpn_running"
        private const val TAG       = "ClashVpnService"
        private const val CHANNEL_ID = "clashforge_vpn"
        private const val NOTIF_ID   = 1001
        private val DEFAULT_DOH_PROBE_CANDIDATES = listOf(
            "https://1.1.1.1/dns-query",
            "https://8.8.8.8/dns-query",
            "https://doh.pub/dns-query",
            "https://dns.alidns.com/dns-query",
        )
        private val DEFAULT_BOOTSTRAP_NAMESERVERS = listOf("1.1.1.1", "8.8.8.8")

        @Volatile var vpnRunning     = false
        @Volatile var mihomoRunning  = false
        @Volatile var mihomoPid      = -1
    }


    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> startVpn()
            ACTION_STOP  -> stopVpn()
        }
        return START_STICKY
    }

    @Synchronized
    private fun startVpn() {
        if (isRunning) {
            LogEventBridge.warn("vpn", "startVpn called but already running")
            return
        }
        isRunning  = true
        vpnRunning = true
        createNotificationChannel()
        val notif = buildVpnNotification()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(NOTIF_ID, notif, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIF_ID, notif)
        }
        sendVpnStateBroadcast(true)
        LogEventBridge.info("vpn", "Starting VPN thread")
        vpnThread = Thread(this, "ClashVpnThread").apply { start() }
    }

    @Synchronized
    private fun stopVpn() {
        if (!isRunning) {
            LogEventBridge.warn("vpn", "stopVpn called but not running")
            return
        }
        isRunning     = false
        vpnRunning    = false
        mihomoRunning = false
        mihomoPid     = -1

        @Suppress("DEPRECATION")
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            stopForeground(Service.STOP_FOREGROUND_REMOVE)
        } else {
            stopForeground(true)
        }
        sendVpnStateBroadcast(false)

        coreProcess?.destroy()
        coreProcess = null
        LogEventBridge.debug("vpn", "Mihomo process destroyed")

        try {
            vpnInterface?.close()
        } catch (e: Exception) {
            Log.e(TAG, "Error closing VPN interface", e)
            LogEventBridge.error("vpn", "Error closing VPN interface: ${e.message}")
        }
        vpnInterface = null
        stopSelf()
        LogEventBridge.info("vpn", "VPN stopped")
    }

    override fun run() {
        try {
            extractAssetsIfNeeded()
            if (!shouldUseCiTorConfig()) {
                // Probe before establishing VPN interface; otherwise probe sockets may
                // be routed back into the tunnel and produce false negatives.
                val configFile = File(filesDir, "config.yaml")
                if (configFile.exists()) {
                    patchConfigForUpstreamFakeIP(configFile)
                }
            }

            LogEventBridge.info("vpn", "Building VPN interface",
                mapOf("addr" to "172.19.0.1/30", "route" to "0.0.0.0/0", "dns" to "172.19.0.2"))

            val builder = Builder()
                .addAddress("172.19.0.1", 30)
                .addRoute("0.0.0.0", 0)
                // DNS must NOT be 172.19.0.1 (the tun0 interface's own address).
                // The kernel's local routing table (priority 0) intercepts packets to
                // the interface's own IP before any VPN routing rules apply, so DNS
                // queries to 172.19.0.1:53 never reach the TUN fd and mihomo's
                // dns-hijack never sees them.  172.19.0.2 is the peer address in the
                // same /30 subnet — not in the local table — so queries to it are
                // forwarded through tun0 → TUN fd → gVisor → dns-hijack correctly.
                .addDnsServer("172.19.0.2")
                .setMtu(1500)                 // explicit MTU so sing-tun doesn't need to set it
                .setSession("ClashForge")
                .setBlocking(false)

            // Prevent self-capture loops: core outbound sockets (DNS/DoH/proxy upstream)
            // must bypass this VPN tunnel.
            try {
                builder.addDisallowedApplication(packageName)
                LogEventBridge.info("vpn", "Excluded app from VPN routing", mapOf("package" to packageName))
            } catch (e: Exception) {
                LogEventBridge.warn("vpn", "Failed to exclude app from VPN routing", mapOf("error" to (e.message ?: "unknown")))
            }

            vpnInterface = builder.establish() ?: run {
                LogEventBridge.error("vpn", "builder.establish() returned null")
                return
            }

            val tunFd = vpnInterface!!.fd
            LogEventBridge.info("vpn", "VPN interface established", mapOf("fd" to tunFd))

            // Android closes ALL fds >= 3 in the child before exec (closeDescriptors).
            // Only fds 0/1/2 survive.  We dup the TUN fd onto fd 0 (stdin) in the parent
            // before forking, tell ProcessBuilder to INHERIT stdin, then restore fd 0.
            // POSIX dup2 clears FD_CLOEXEC on the new fd, so fd 0 survives exec.
            // sing-tun is built from source with the sentinel changed from 0 to -1, so
            // file-descriptor: 0 means "use fd 0" and mihomo's stdout (fd 1) stays
            // connected to our log pipe — no interference with TUN reads/writes.
            patchConfigWithTun(0)

            startMihomoCore(tunFd)

            LogEventBridge.info("vpn", "VPN loop running")
            while (isRunning) Thread.sleep(1000)

        } catch (e: Exception) {
            Log.e(TAG, "VPN run error", e)
            LogEventBridge.error("vpn", "VPN run() exception: ${e.message}",
                mapOf("type" to e.javaClass.simpleName))
        } finally {
            stopVpn()
        }
    }

    // Returns true only when CI Tor mode is explicitly enabled via a marker file
    // and the forwarded Tor SOCKS5 endpoint is reachable.
    // Marker: /data/local/tmp/clashforge_ci_tor.enable
    private fun shouldUseCiTorConfig(): Boolean {
        val marker = File("/data/local/tmp/clashforge_ci_tor.enable")
        if (!marker.exists()) {
            return false
        }
        return isTorPortReachable()
    }

    private fun isTorPortReachable(): Boolean = try {
        java.net.Socket().use {
            it.connect(java.net.InetSocketAddress("127.0.0.1", 9050), 300)
        }
        true
    } catch (_: Exception) { false }

    // In CI, subscription proxy nodes are blocked from GitHub Actions (Azure) IPs.
    // When Tor is detected on 127.0.0.1:9050 (forwarded from the CI host via adb reverse),
    // we replace the entire config with a minimal one that uses Tor as the sole proxy.
    // This guarantees a different exit IP without depending on external proxy reachability.
    private fun buildCiConfig(tunFd: Int) = """
port: 7890
socks-port: 7891
allow-lan: false
mode: rule
log-level: debug
external-controller: 127.0.0.1:9090

dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  nameserver:
    - 8.8.8.8
    - 1.1.1.1
  fake-ip-filter:
    - "*.lan"

proxies:
  - name: ci-tor
    type: socks5
    server: 127.0.0.1
    port: 9050

proxy-groups:
  - name: Proxy
    type: select
    proxies:
      - ci-tor

rules:
  - GEOIP,private,DIRECT,no-resolve
  - MATCH,Proxy

tun:
  enable: true
  stack: gvisor
  file-descriptor: $tunFd
  auto-route: false
  auto-detect-interface: false
  dns-hijack:
    - "any:53"
    - "tcp://any:53"
    - "tls://any:853"

sniffer:
  enable: true
  override-destination: true
  parse-pure-ip: true
  sniff:
    TLS:
      ports: [443, 8443]
    HTTP:
      ports: [80, 8080-8880]
    QUIC:
      ports: [443]
""".trimIndent()

    private fun patchConfigWithTun(tunFd: Int) {
        val configFile = File(filesDir, "config.yaml")

        if (shouldUseCiTorConfig()) {
            LogEventBridge.info("vpn", "CI mode: Tor SOCKS5 detected on :9050 — writing CI config", mapOf("tunFd" to tunFd))
            configFile.writeText(buildCiConfig(tunFd))
            return
        }

        if (!configFile.exists()) {
            LogEventBridge.warn("vpn", "config.yaml not found — skipping TUN patch")
            return
        }

        val original = try {
            configFile.readText()
        } catch (e: Exception) {
            LogEventBridge.warn("vpn", "Read config.yaml failed before TUN patch", mapOf("error" to (e.message ?: "unknown")))
            return
        }

        // Keep DNS bootstrap/direct lookups out of proxy rules to avoid resolver loops.
        // Also force fake-ip mode so Chrome/browsers never see GFW-poisoned DNS responses:
        // mihomo returns a synthetic 198.18.x.x IP immediately, Chrome connects to it,
        // mihomo maps it to the domain and forwards to the proxy which does remote DNS.
        // This migration runs at every VPN start so it applies to configs written by older
        // app versions that still had enhanced-mode: redir-host.
        var dnsPatched = upsertDnsScalar(original, "respect-rules", "false")
        dnsPatched = upsertDnsScalar(dnsPatched, "enhanced-mode", "fake-ip")
        dnsPatched = upsertDnsScalar(dnsPatched, "fake-ip-range", "198.18.0.0/15")
        dnsPatched = upsertDnsList(dnsPatched, "fake-ip-filter", listOf(
            "*.lan", "*.local", "*.localhost", "*.localdomain",
            "+.stun.*.*", "+.stun.*.*.*",
            "msftconnecttest.com", "*.msftconnecttest.com",
            "time.*.com", "ntp.*.com", "*.pool.ntp.org",
        ))

        // Make this patch idempotent: remove any existing top-level tun/sniffer blocks
        // before appending the canonical VPN stanza.
        val withoutTun = removeTopLevelSection(dnsPatched, "tun")
        val sanitized = removeTopLevelSection(withoutTun, "sniffer").trimEnd()

        // Append TUN + sniffer together.
        // sniffer enables QUIC/TLS/HTTP domain identification so mihomo can apply
        // domain-based rules to QUIC (HTTP/3) connections via the gvisor TUN stack.
        // Without sniffer, QUIC to HTTP-proxy nodes causes a silent timeout before
        // the app falls back to TCP; with sniffer mihomo identifies the domain and
        // can fast-fail unsupported UDP sessions.
        // If the subscription config already has a sniffer block this override is
        // intentional — our config is always more complete for VPN usage.
        // file-descriptor: 0 = mihomo's fd 0 (stdin), which startMihomoCore temporarily
        // replaces with the TUN fd via Os.dup2 before forking.  sing-tun is patched to
        // treat FileDescriptor<0 (not 0) as "not set", so 0 means use fd 0 directly.
        // sing-tun.New() skips configure() entirely when its sentinel check is false.
        // Append TUN + sniffer.
        // ConfigGenerator already writes a dns: block (fake-ip) — appending another
        // would cause a YAML duplicate key error and a mihomo fatal parse failure.
        val tunStanza = """

tun:
  enable: true
  stack: gvisor
  file-descriptor: $tunFd
  auto-route: false
  auto-detect-interface: false
  dns-hijack:
    - "any:53"
    - "tcp://any:53"
    - "tls://any:853"

sniffer:
  enable: true
  override-destination: true
  parse-pure-ip: true
  sniff:
    TLS:
      ports: [443, 8443]
    HTTP:
      ports: [80, 8080-8880]
    QUIC:
      ports: [443]
"""
        configFile.writeText(sanitized + tunStanza)
        LogEventBridge.info("vpn", "Patched config.yaml with TUN fd", mapOf(
            "file-descriptor" to tunFd,
            "stack"           to "gvisor",
            "note"            to "stdin(fd0)=TUN"
        ))
    }

    private fun removeTopLevelSection(config: String, key: String): String {
        val lines = config.replace("\r\n", "\n").split('\n').toMutableList()
        var i = 0
        while (i < lines.size) {
            val line = lines[i]
            if (line.trim() == "$key:" && !line.startsWith(" ")) {
                var end = i + 1
                while (end < lines.size) {
                    val ln = lines[end]
                    if (ln.isNotEmpty() && !ln.startsWith(" ")) {
                        break
                    }
                    end++
                }
                lines.subList(i, end).clear()
                continue
            }
            i++
        }
        return lines.joinToString("\n")
    }

    private fun patchConfigForUpstreamFakeIP(configFile: File) {
        val original = try {
            configFile.readText()
        } catch (e: Exception) {
            LogEventBridge.warn("dns", "Read config.yaml failed, skip dns probe", mapOf("error" to (e.message ?: "unknown")))
            return
        }

        val sampleHosts = extractProxyHostnames(original).distinct().take(3)
        if (sampleHosts.isEmpty()) {
            LogEventBridge.info("dns", "Skip upstream DNS probe: no proxy hostnames")
            return
        }

        val udpNameservers = extractDnsList(original, "nameserver")
            .map { it.trim() }
            .filter { isUdpNameserver(it) }
            .distinct()

        if (udpNameservers.isEmpty()) {
            LogEventBridge.info("dns", "Skip upstream DNS probe: no UDP nameserver configured")
            return
        }

        LogEventBridge.info("dns", "Probing upstream DNS fake-ip hijack", mapOf(
            "nameserver_count" to udpNameservers.size,
            "hostname_count" to sampleHosts.size,
        ))

        val hijacked = linkedSetOf<String>()
        val working = linkedSetOf<String>()
        val unresolved = linkedSetOf<String>()
        val hostHasUsableAnswer = sampleHosts.associateWith { false }.toMutableMap()

        for (ns in udpNameservers) {
            var nsHijacked = false
            var nsHasFailure = false
            for (hostname in sampleHosts) {
                val ips = queryUdpA(ns, hostname)
                if (ips.isEmpty()) {
                    nsHasFailure = true
                    continue
                }
                if (allInKnownFakeRanges(ips)) {
                    nsHijacked = true
                    break
                }
                hostHasUsableAnswer[hostname] = true
            }
            if (nsHijacked) {
                hijacked += ns
            } else {
                if (nsHasFailure) {
                    unresolved += ns
                } else {
                    working += ns
                }
            }
        }

        val unresolvedHosts = hostHasUsableAnswer
            .filterValues { !it }
            .keys
            .toList()
        val hasUnusableHost = unresolvedHosts.isNotEmpty()
        val allUdpUnresolved = working.isEmpty() && unresolved.isNotEmpty()

        if (hijacked.isEmpty() && !hasUnusableHost) {
            LogEventBridge.info("dns", "Upstream DNS probe passed", mapOf(
                "working_nameservers" to working.joinToString(","),
                "unresolved_nameservers" to unresolved.joinToString(","),
            ))
            return
        }

        if (allUdpUnresolved || hasUnusableHost) {
            LogEventBridge.warn("dns", "All UDP nameservers failed to resolve proxy hostnames; fallback to DoH-only", mapOf(
                "unresolved_nameservers" to unresolved.joinToString(","),
                "unresolved_hosts" to unresolvedHosts.joinToString(","),
                "hostname_count" to sampleHosts.size,
            ))
        }

        val probeHost = sampleHosts.first()
        val suggestedDoH = mutableListOf<String>()
        for (doh in DEFAULT_DOH_PROBE_CANDIDATES) {
            val ips = queryDohA(doh, probeHost)
            if (ips.isNotEmpty() && !allInKnownFakeRanges(ips)) {
                suggestedDoH += doh
            }
        }

        val existingProxyServerNS = extractDnsList(original, "proxy-server-nameserver")
        val existingDoh = existingProxyServerNS
            .map { it.trim() }
            .filter { isDohNameserver(it) }
            .map { withSkipCertVerifyParam(it) }
            .distinct()
        val existingIpLiteralDoh = existingDoh.filter { isIpLiteralDoh(it) }

        val verifiedDoH = suggestedDoH
            .map { withSkipCertVerifyParam(it) }
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .distinct()
        val verifiedIpLiteralDoh = verifiedDoH.filter { isIpLiteralDoh(it) }

        val builtInDoH = DEFAULT_DOH_PROBE_CANDIDATES
            .map { withSkipCertVerifyParam(it) }
            .distinct()
        val builtInIpLiteralDoh = builtInDoH.filter { isIpLiteralDoh(it) }

        val finalDoH = when {
            verifiedIpLiteralDoh.isNotEmpty() -> (existingIpLiteralDoh + verifiedIpLiteralDoh).distinct()
            verifiedDoH.isNotEmpty() -> (existingDoh + verifiedDoH).distinct()
            existingIpLiteralDoh.isNotEmpty() -> existingIpLiteralDoh
            builtInIpLiteralDoh.isNotEmpty() -> builtInIpLiteralDoh
            else -> (existingDoh + builtInDoH).distinct()
        }

        if (finalDoH.isEmpty()) {
            LogEventBridge.warn("dns", "Detected upstream DNS issue but no DoH candidates available", mapOf(
                "hijacked_nameservers" to hijacked.joinToString(","),
                "unresolved_nameservers" to unresolved.joinToString(","),
            ))
            return
        }

        if (verifiedDoH.isEmpty()) {
            LogEventBridge.warn("dns", "No verified DoH during probe; using best-effort DoH candidate set", mapOf(
                "hijacked_nameservers" to hijacked.joinToString(","),
                "unresolved_nameservers" to unresolved.joinToString(","),
                "auto_applied_doh" to finalDoH.joinToString(","),
            ))
        }

        // Align with OpenWrt startup self-healing: once upstream DNS is deemed
        // polluted/unusable, force DNS to encrypted resolvers to avoid carrying
        // over stale domain-based fallback entries from previous runs.
        val finalNameserver = finalDoH
        val finalProxyServerNS = finalDoH
        val finalFallback = finalDoH.filter { isIpLiteralDoh(it) }.ifEmpty { finalDoH }

        val existingDefaultNS = extractDnsList(original, "default-nameserver")
            .mapNotNull { extractIpLiteralFromPlainNameserver(it) }
        val workingDefaultNS = working
            .mapNotNull { extractIpLiteralFromPlainNameserver(it) }
        val dohDefaultNS = finalDoH
            .mapNotNull { extractIpLiteralHostFromDoh(it) }
        val finalDefaultNS = (dohDefaultNS + existingDefaultNS + workingDefaultNS + DEFAULT_BOOTSTRAP_NAMESERVERS)
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .distinct()

        var patched = upsertDnsList(original, "nameserver", finalNameserver)
        patched = upsertDnsList(patched, "proxy-server-nameserver", finalProxyServerNS)
        patched = upsertDnsList(patched, "fallback", finalFallback)
        if (finalDefaultNS.isNotEmpty()) {
            patched = upsertDnsList(patched, "default-nameserver", finalDefaultNS)
        }

        if (patched == original) {
            LogEventBridge.warn("dns", "Detected upstream fake-ip hijack but failed to patch DNS resolvers", mapOf(
                "hijacked_nameservers" to hijacked.joinToString(",")
            ))
            return
        }

        try {
            configFile.writeText(patched)
            LogEventBridge.warn("dns", "Upstream fake-ip hijack detected; switched DNS to DoH-only set", mapOf(
                "hijacked_nameservers" to hijacked.joinToString(","),
                "working_nameservers" to working.joinToString(","),
                "unresolved_nameservers" to unresolved.joinToString(","),
                "unresolved_hosts" to unresolvedHosts.joinToString(","),
                "existing_doh" to existingDoh.joinToString(","),
                "auto_applied_doh" to verifiedDoH.joinToString(","),
                "final_nameserver" to finalNameserver.joinToString(","),
                "final_proxy_server_nameserver" to finalProxyServerNS.joinToString(","),
                "final_fallback" to finalFallback.joinToString(","),
                "final_default_nameserver" to finalDefaultNS.joinToString(","),
            ))
        } catch (e: Exception) {
            LogEventBridge.warn("dns", "Patch config.yaml failed after DNS probe", mapOf("error" to (e.message ?: "unknown")))
        }
    }

    private fun isDohNameserver(ns: String): Boolean {
        val lower = ns.trim().lowercase(Locale.ROOT)
        return lower.startsWith("https://")
    }

    private fun isIpLiteralDoh(ns: String): Boolean {
        if (!isDohNameserver(ns)) return false
        return try {
            val host = URL(ns.substringBefore('#')).host
            isIpLiteralHost(host)
        } catch (_: Exception) {
            false
        }
    }

    private fun extractIpLiteralHostFromDoh(doh: String): String? {
        if (!isDohNameserver(doh)) return null
        return try {
            val host = URL(doh.substringBefore('#')).host
            if (isIpLiteralHost(host)) host else null
        } catch (_: Exception) {
            null
        }
    }

    private fun extractIpLiteralFromPlainNameserver(nameserver: String): String? {
        if (nameserver.isBlank()) return null
        if (isDohNameserver(nameserver)) return null

        var serverPart = nameserver.trim()
        val lower = serverPart.lowercase(Locale.ROOT)
        if (lower.startsWith("dhcp://")) return null
        if (lower.startsWith("udp://") || lower.startsWith("tcp://") || lower.startsWith("tls://")) {
            serverPart = serverPart.substringAfter("://")
        }

        val host = when {
            serverPart.startsWith("[") -> {
                val end = serverPart.indexOf(']')
                if (end > 0) serverPart.substring(1, end) else serverPart
            }
            serverPart.count { it == ':' } > 1 -> serverPart
            serverPart.contains(':') -> serverPart.substringBeforeLast(':')
            else -> serverPart
        }.trim()

        return if (isIpLiteralHost(host)) host else null
    }

    private fun isIpLiteralHost(host: String): Boolean {
        val normalized = host.trim().removePrefix("[").removeSuffix("]")
        if (normalized.isEmpty()) return false
        return normalized.matches(Regex("""^\d{1,3}(\.\d{1,3}){3}$""")) || normalized.contains(':')
    }

    private fun withSkipCertVerifyParam(doh: String): String {
        val raw = doh.trim()
        if (raw.isEmpty()) return raw
        val lower = raw.lowercase(Locale.ROOT)
        if (lower.contains("skip-cert-verify=")) return raw
        return if (raw.contains("#")) {
            "$raw&skip-cert-verify=true"
        } else {
            "$raw#skip-cert-verify=true"
        }
    }

    private fun isUdpNameserver(ns: String): Boolean {
        if (ns.isBlank()) return false
        val lower = ns.lowercase(Locale.ROOT)
        return !lower.startsWith("https://")
            && !lower.startsWith("tls://")
            && !lower.startsWith("tcp://")
            && !lower.startsWith("dhcp://")
    }

    private fun extractProxyHostnames(config: String): List<String> {
        val out = mutableListOf<String>()
        val lines = config.replace("\r\n", "\n").split('\n')
        var inProxies = false

        for (line in lines) {
            val trimmed = line.trim()
            val topLevel = line.isNotEmpty() && !line.startsWith(" ")
            if (topLevel) {
                inProxies = trimmed == "proxies:"
                continue
            }
            if (!inProxies || trimmed.isEmpty()) continue
            if (!trimmed.startsWith("server:")) continue
            val value = parseYamlScalar(trimmed.substringAfter(':'))
            if (isLikelyHostname(value)) {
                out += value
            }
        }
        return out
    }

    private fun extractDnsList(config: String, key: String): List<String> {
        val lines = config.replace("\r\n", "\n").split('\n')
        val dnsStart = lines.indexOfFirst { it.trim() == "dns:" && !it.startsWith(" ") }
        if (dnsStart < 0) return emptyList()

        var dnsEnd = lines.size
        for (i in dnsStart + 1 until lines.size) {
            val ln = lines[i]
            if (ln.isNotEmpty() && !ln.startsWith(" ")) {
                dnsEnd = i
                break
            }
        }

        val keyLine = lines.subList(dnsStart + 1, dnsEnd)
            .indexOfFirst { it.trim() == "$key:" }
            .let { if (it < 0) -1 else dnsStart + 1 + it }
        if (keyLine < 0) return emptyList()

        val out = mutableListOf<String>()
        for (i in keyLine + 1 until dnsEnd) {
            val ln = lines[i]
            val trimmed = ln.trim()
            if (trimmed.isEmpty()) continue
            if (!ln.startsWith("    ")) break
            if (!trimmed.startsWith("-")) continue
            val value = parseYamlScalar(trimmed.removePrefix("-").trim())
            if (value.isNotEmpty()) {
                out += value
            }
        }
        return out
    }

    private fun upsertDnsList(config: String, key: String, values: List<String>): String {
        val lines = config.replace("\r\n", "\n").split('\n').toMutableList()
        val dnsStart = lines.indexOfFirst { it.trim() == "dns:" && !it.startsWith(" ") }
        if (dnsStart < 0) return config

        var dnsEnd = lines.size
        for (i in dnsStart + 1 until lines.size) {
            val ln = lines[i]
            if (ln.isNotEmpty() && !ln.startsWith(" ")) {
                dnsEnd = i
                break
            }
        }

        val block = mutableListOf("  $key:")
        for (v in values) {
            // '*' and '+' are YAML special chars (alias/merge) at start of a scalar value
            val rendered = if (v.any { it.isWhitespace() || it == ':' || it == '#' || it == '"' || it == '\'' || it == '*' || it == '+' }) {
                "\"" + v.replace("\\", "\\\\").replace("\"", "\\\"") + "\""
            } else {
                v
            }
            block += "    - $rendered"
        }

        var keyStart = -1
        for (i in dnsStart + 1 until dnsEnd) {
            if (lines[i].trim() == "$key:") {
                keyStart = i
                break
            }
        }

        if (keyStart >= 0) {
            var keyEnd = keyStart + 1
            while (keyEnd < dnsEnd) {
                val ln = lines[keyEnd]
                if (ln.isNotEmpty() && !ln.startsWith("    ")) break
                keyEnd++
            }
            lines.subList(keyStart, keyEnd).clear()
            lines.addAll(keyStart, block)
        } else {
            lines.addAll(dnsEnd, block)
        }

        return lines.joinToString("\n")
    }

    private fun upsertDnsScalar(config: String, key: String, value: String): String {
        val lines = config.replace("\r\n", "\n").split('\n').toMutableList()
        val dnsStart = lines.indexOfFirst { it.trim() == "dns:" && !it.startsWith(" ") }
        if (dnsStart < 0) return config

        var dnsEnd = lines.size
        for (i in dnsStart + 1 until lines.size) {
            val ln = lines[i]
            if (ln.isNotEmpty() && !ln.startsWith(" ")) {
                dnsEnd = i
                break
            }
        }

        val scalarLine = "  $key: $value"
        var keyStart = -1
        for (i in dnsStart + 1 until dnsEnd) {
            if (lines[i].trim().startsWith("$key:")) {
                keyStart = i
                break
            }
        }

        if (keyStart >= 0) {
            var keyEnd = keyStart + 1
            while (keyEnd < dnsEnd) {
                val ln = lines[keyEnd]
                if (ln.isNotEmpty() && !ln.startsWith("    ")) break
                keyEnd++
            }
            lines.subList(keyStart, keyEnd).clear()
            lines.add(keyStart, scalarLine)
        } else {
            lines.add(dnsStart + 1, scalarLine)
        }

        return lines.joinToString("\n")
    }

    private fun parseYamlScalar(raw: String): String {
        val v = raw.trim()
        if (v.length >= 2 && v.startsWith('"') && v.endsWith('"')) {
            return v.substring(1, v.length - 1)
                .replace("\\\"", "\"")
                .replace("\\\\", "\\")
        }
        return v
    }

    private fun isLikelyHostname(host: String): Boolean {
        if (host.isBlank()) return false
        if (host.startsWith("[")) return false
        return !host.matches(Regex("""^\d{1,3}(\.\d{1,3}){3}$"""))
    }

    private fun queryUdpA(nameserver: String, hostname: String): List<String> {
        val serverPart = nameserver.removePrefix("udp://")
        val (host, port) = when {
            serverPart.startsWith("[") -> {
                val end = serverPart.indexOf(']')
                if (end > 0) {
                    val h = serverPart.substring(1, end)
                    val p = if (end + 1 < serverPart.length && serverPart[end + 1] == ':') {
                        serverPart.substring(end + 2).toIntOrNull() ?: 53
                    } else {
                        53
                    }
                    h to p
                } else {
                    serverPart to 53
                }
            }
            serverPart.count { it == ':' } > 1 -> serverPart to 53
            serverPart.contains(':') -> {
                val idx = serverPart.lastIndexOf(':')
                val h = serverPart.substring(0, idx)
                val p = serverPart.substring(idx + 1).toIntOrNull() ?: 53
                h to p
            }
            else -> serverPart to 53
        }

        return try {
            val queryId = ThreadLocalRandom.current().nextInt(0, 65536)
            val query = buildDnsQuery(hostname, queryId)
            DatagramSocket().use { socket ->
                socket.soTimeout = 5000
                val packet = DatagramPacket(query, query.size, InetAddress.getByName(host), port)
                socket.send(packet)

                val buf = ByteArray(1500)
                val resp = DatagramPacket(buf, buf.size)
                socket.receive(resp)
                parseDnsAAnswers(buf, resp.length, queryId)
            }
        } catch (e: Exception) {
            LogEventBridge.debug("dns", "UDP DNS probe failed", mapOf(
                "nameserver" to nameserver,
                "hostname" to hostname,
                "error" to (e.message ?: "unknown")
            ))
            emptyList()
        }
    }

    private fun queryDohA(dohURL: String, hostname: String): List<String> {
        return try {
            val endpoint = URL("$dohURL?name=$hostname&type=A")
            val conn = endpoint.openConnection() as HttpURLConnection
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            conn.requestMethod = "GET"
            conn.setRequestProperty("Accept", "application/dns-json")
            conn.instanceFollowRedirects = true

            if (conn.responseCode != HttpURLConnection.HTTP_OK) {
                conn.disconnect()
                return emptyList()
            }

            val body = conn.inputStream.bufferedReader().use { it.readText() }
            conn.disconnect()
            val obj = JSONObject(body)
            val answers = obj.optJSONArray("Answer") ?: return emptyList()
            val ips = mutableListOf<String>()
            for (i in 0 until answers.length()) {
                val ans = answers.optJSONObject(i) ?: continue
                if (ans.optInt("type") == 1) {
                    val ip = ans.optString("data", "")
                    if (ip.isNotBlank()) ips += ip
                }
            }
            ips
        } catch (e: Exception) {
            LogEventBridge.debug("dns", "DoH probe failed", mapOf(
                "doh" to dohURL,
                "hostname" to hostname,
                "error" to (e.message ?: "unknown")
            ))
            emptyList()
        }
    }

    private fun buildDnsQuery(hostname: String, queryId: Int): ByteArray {
        val out = ByteArrayOutputStream()
        out.write(byteArrayOf(
            ((queryId ushr 8) and 0xFF).toByte(), (queryId and 0xFF).toByte(),
            0x01, 0x00, // RD=1 standard query
            0x00, 0x01, // QDCOUNT
            0x00, 0x00, // ANCOUNT
            0x00, 0x00, // NSCOUNT
            0x00, 0x00, // ARCOUNT
        ))

        for (label in hostname.trim('.').split('.')) {
            if (label.isEmpty()) continue
            val bytes = label.toByteArray(Charsets.US_ASCII)
            out.write(bytes.size)
            out.write(bytes)
        }
        out.write(0x00)        // QNAME terminator
        out.write(0x00); out.write(0x01) // QTYPE A
        out.write(0x00); out.write(0x01) // QCLASS IN
        return out.toByteArray()
    }

    private fun parseDnsAAnswers(data: ByteArray, length: Int, queryId: Int): List<String> {
        if (length < 12) return emptyList()
        val msg = data.copyOf(length)
        val respId = u16(msg, 0)
        if (respId != queryId) return emptyList()

        val qdCount = u16(msg, 4)
        val anCount = u16(msg, 6)

        var offset = 12
        repeat(qdCount) {
            offset = skipDnsName(msg, offset)
            if (offset + 4 > msg.size) return emptyList()
            offset += 4
        }

        val ips = mutableListOf<String>()
        repeat(anCount) {
            offset = skipDnsName(msg, offset)
            if (offset + 10 > msg.size) return@repeat

            val type = u16(msg, offset)
            val klass = u16(msg, offset + 2)
            val rdLen = u16(msg, offset + 8)
            offset += 10

            if (offset + rdLen > msg.size) return@repeat

            if (type == 1 && klass == 1 && rdLen == 4) {
                val ipBytes = msg.copyOfRange(offset, offset + 4)
                val hostAddress = InetAddress.getByAddress(ipBytes).hostAddress
                if (!hostAddress.isNullOrBlank()) {
                    ips += hostAddress
                }
            }
            offset += rdLen
        }
        return ips
    }

    private fun skipDnsName(msg: ByteArray, start: Int): Int {
        var p = start
        while (p < msg.size) {
            val len = msg[p].toInt() and 0xFF
            if (len == 0) {
                return p + 1
            }
            if ((len and 0xC0) == 0xC0) {
                return p + 2
            }
            p += len + 1
        }
        return p
    }

    private fun u16(buf: ByteArray, offset: Int): Int {
        if (offset + 1 >= buf.size) return 0
        return ((buf[offset].toInt() and 0xFF) shl 8) or (buf[offset + 1].toInt() and 0xFF)
    }

    private fun allInKnownFakeRanges(ips: List<String>): Boolean {
        if (ips.isEmpty()) return false
        return ips.all { isKnownFakeIP(it) }
    }

    private fun isKnownFakeIP(ip: String): Boolean {
        val addr = try {
            InetAddress.getByName(ip).address
        } catch (_: Exception) {
            return false
        }
        if (addr.size != 4) return false

        val b0 = addr[0].toInt() and 0xFF
        val b1 = addr[1].toInt() and 0xFF

        // 198.18.0.0/15 and 28.0.0.0/8
        return (b0 == 198 && (b1 == 18 || b1 == 19)) || b0 == 28
    }

    private fun extractAssetsIfNeeded() {
        val markerFile = File(filesDir, ".asset_version")
        val currentVersion = packageManager
            .getPackageInfo(packageName, 0)
            .let {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P)
                    it.longVersionCode.toString()
                else
                    @Suppress("DEPRECATION") it.versionCode.toString()
            }

        if (markerFile.exists() && markerFile.readText().trim() == currentVersion) {
            LogEventBridge.debug("assets", "Assets up-to-date", mapOf("version" to currentVersion))
            return
        }

        LogEventBridge.info("assets", "Extracting assets", mapOf("version" to currentVersion))

        extractAsset("flutter_assets/assets/geodata/country.mmdb", File(filesDir, "country.mmdb"))
        extractAsset("flutter_assets/assets/geodata/geosite.dat",  File(filesDir, "geosite.dat"))

        markerFile.writeText(currentVersion)
        LogEventBridge.info("assets", "Extraction complete")
    }

    private fun extractAsset(assetPath: String, dest: File, executable: Boolean = false) {
        try {
            assets.open(assetPath).use { input ->
                FileOutputStream(dest).use { output -> input.copyTo(output) }
            }
            if (executable) dest.setExecutable(true)
            LogEventBridge.debug("assets", "Extracted ${dest.name}",
                mapOf("size" to dest.length(), "path" to dest.absolutePath))
        } catch (e: Exception) {
            Log.e(TAG, "Failed to extract $assetPath", e)
            LogEventBridge.error("assets", "Extract failed: ${dest.name}",
                mapOf("asset" to assetPath, "error" to (e.message ?: "unknown")))
        }
    }

    private fun startMihomoCore(tunFd: Int) {
        val appDir     = filesDir.absolutePath
        // nativeLibraryDir is on an executable partition; filesDir is mounted noexec on API 29+
        val coreBin    = File(applicationInfo.nativeLibraryDir, "libmihomo.so")
        val configFile = File(appDir, "config.yaml")

        if (!coreBin.exists()) {
            LogEventBridge.error("mihomo", "Binary not found", mapOf("path" to coreBin.absolutePath))
            return
        }
        coreBin.setExecutable(true)

        LogEventBridge.info("mihomo", "Starting core", mapOf(
            "bin"           to coreBin.absolutePath,
            "bin_size_mb"   to String.format("%.1f", coreBin.length() / 1048576.0),
            "config_exists" to configFile.exists(),
            "config_size"   to configFile.length(),
            "abi"           to (android.os.Build.SUPPORTED_ABIS.firstOrNull() ?: "unknown"),
            "tun_fd"        to tunFd
        ))

        val tunFdObj = vpnInterface?.fileDescriptor ?: run {
            LogEventBridge.error("mihomo", "vpnInterface null before spawn")
            return
        }

        // Android closes ALL fds >= 3 in the child before exec (closeDescriptors).
        // The only fds that survive into mihomo are 0 (stdin), 1 (stdout), 2 (stderr).
        //
        // Strategy: dup the TUN fd onto fd 0 (stdin) in THIS process, fork with
        // INHERIT for stdin so the child gets fd 0 = TUN, then restore fd 0 here.
        // POSIX dup2() always clears FD_CLOEXEC on the new fd, so fd 0 survives exec.
        // sing-tun is patched (FileDescriptor<0 = "not set") so file-descriptor: 0
        // means "use fd 0".  mihomo's stdout (fd 1) and stderr (fd 2) stay connected
        // to normal pipes — no write errors, no interference with TUN reads.
        val savedStdin: FileDescriptor = Os.dup(FileDescriptor.`in`)
        try {
            Os.dup2(tunFdObj, 0)   // fd 0 in this process = TUN fd

            val pb = ProcessBuilder(coreBin.absolutePath, "-d", appDir, "-f", configFile.absolutePath)
            pb.environment().apply {
                // MADV_DONTNEED: return freed pages to the kernel immediately instead of
                // MADV_FREE (default), which keeps pages mapped until kernel needs them.
                // Eliminates ~300 MB of "Private Clean" PSS that would otherwise sit in
                // the process address space until memory pressure forces a reclaim.
                put("GODEBUG",    "madvdontneed=1")
                // GC at 40% heap growth (default 100%) — reduces peak live-object dirty pages.
                put("GOGC",       "40")
                // Soft memory cap: runtime scavenges more aggressively to stay under limit.
                // Current Private Dirty is ~56 MB; 100 MiB gives comfortable headroom.
                put("GOMEMLIMIT", "100MiB")
            }
            pb.redirectInput(ProcessBuilder.Redirect.INHERIT)  // fd 0 = TUN (no dup2 by JVM)
            pb.redirectErrorStream(true)                       // merge stderr into stdout pipe
            coreProcess = pb.start()
        } finally {
            Os.dup2(savedStdin, 0)   // restore parent's stdin regardless of outcome
            Os.close(savedStdin)
        }

        val pid = try {
            val f = coreProcess!!.javaClass.getDeclaredField("pid")
            f.isAccessible = true
            f.getInt(coreProcess)
        } catch (_: Exception) { -1 }

        mihomoPid     = pid
        mihomoRunning = true
        LogEventBridge.info("mihomo", "Core started", mapOf("pid" to pid, "tun_fd" to tunFd))

        Thread {
            try {
                coreProcess?.inputStream?.bufferedReader()?.forEachLine { line ->
                    Log.d("MihomoCore", line)
                    LogEventBridge.debug("mihomo", line)
                }
                val exit = coreProcess?.exitValue() ?: -1
                mihomoRunning = false
                mihomoPid     = -1
                LogEventBridge.warn("mihomo", "Core exited", mapOf("exit_code" to exit))
            } catch (e: Exception) {
                LogEventBridge.error("mihomo", "Log reader error: ${e.message}")
            }
        }.start()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "ClashForge VPN",
                NotificationManager.IMPORTANCE_DEFAULT
            ).apply {
                description = "VPN 连接状态"
                setShowBadge(false)
                setSound(null, null)       // silent — status notification, no alert sound
                enableVibration(false)
            }
            (getSystemService(NOTIFICATION_SERVICE) as NotificationManager)
                .createNotificationChannel(channel)
        }
    }

    private fun buildVpnNotification(): Notification {
        val stopIntent = PendingIntent.getService(
            this, 0,
            Intent(this, ClashVpnService::class.java).apply { action = ACTION_STOP },
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val openIntent = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("ClashForge VPN 运行中")
            .setContentText("VPN 已接管网络流量")
            .setSmallIcon(R.drawable.ic_vpn_notification)
            .setContentIntent(openIntent)
            .addAction(0, "断开连接", stopIntent)
            .setOngoing(true)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .build()
    }

    private fun sendVpnStateBroadcast(running: Boolean) {
        sendBroadcast(Intent(ACTION_VPN_STATE_CHANGED).putExtra(EXTRA_VPN_RUNNING, running))
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            TileService.requestListeningState(
                this,
                ComponentName(this, ClashForgeTileService::class.java)
            )
        }
        // On first VPN start, prompt the user to add the Quick Settings tile.
        // requestAddTileService() shows a one-time system sheet; Android ignores it
        // on subsequent calls if the tile is already in the user's QS panel.
        if (running && Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            try {
                val sbm = getSystemService(android.app.StatusBarManager::class.java)
                sbm?.requestAddTileService(
                    ComponentName(this, ClashForgeTileService::class.java),
                    "ClashForge VPN",
                    android.graphics.drawable.Icon.createWithResource(
                        this, R.drawable.ic_vpn_notification
                    ),
                    mainExecutor
                ) { /* result ignored */ }
            } catch (_: Exception) { /* non-fatal: some OEMs don't implement this */ }
        }
    }

    override fun onDestroy() {
        LogEventBridge.info("vpn", "Service onDestroy")
        stopVpn()
        super.onDestroy()
    }
}
