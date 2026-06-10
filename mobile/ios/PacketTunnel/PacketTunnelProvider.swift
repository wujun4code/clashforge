import Foundation
import Mihomobridge
import NetworkExtension

/// iOS counterpart of Android's ClashVpnService.
///
/// Same startup choreography, different mechanics:
///   Android: probe DNS → Builder.establish() (TUN fd) → patch config →
///            fork libmihomo.so with the fd on stdin.
///   iOS:     probe DNS → setTunnelNetworkSettings (Apple creates the utun) →
///            locate the utun fd in our own process → patch config with the
///            fd → start mihomo in-process via the gomobile bridge.
///
/// The tunnel parameters (172.19.0.1/30, DNS 172.19.0.2, default route,
/// MTU 1500) mirror the Android Builder exactly — see ClashVpnService.run()
/// for why DNS must be the /30 peer address rather than the interface's own.
class PacketTunnelProvider: NEPacketTunnelProvider {
    private let logger = ExtensionLogger()
    private lazy var coreLogAdapter = CoreLogAdapter(logger: logger)

    override func startTunnel(options: [String: NSObject]?,
                              completionHandler: @escaping (Error?) -> Void) {
        let home = SharedPaths.containerURL.path
        let configPath = SharedPaths.configURL.path

        logger.info("vpn", "Starting tunnel", fields: ["home": home])

        guard FileManager.default.fileExists(atPath: configPath) else {
            logger.error("vpn", "config.yaml not found — app must connect once to write it")
            completionHandler(NEVPNError(.configurationInvalid))
            return
        }

        // Must run before the tunnel exists; once routes are up the probe
        // sockets would loop through the tunnel and yield false negatives.
        let probeSummary = MihomobridgeProbeAndPatchDNS(configPath)
        logger.info("dns", probeSummary)

        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
        let ipv4 = NEIPv4Settings(addresses: ["172.19.0.1"],
                                  subnetMasks: ["255.255.255.252"])
        ipv4.includedRoutes = [NEIPv4Route.default()]
        settings.ipv4Settings = ipv4
        settings.dnsSettings = NEDNSSettings(servers: ["172.19.0.2"])
        settings.mtu = 1500

        setTunnelNetworkSettings(settings) { [weak self] error in
            guard let self = self else { return }
            if let error = error {
                self.logger.error("vpn", "setTunnelNetworkSettings failed",
                                  fields: ["error": error.localizedDescription])
                completionHandler(error)
                return
            }

            guard let tunFd = self.tunnelFileDescriptor() else {
                self.logger.error("vpn", "Could not locate utun file descriptor")
                completionHandler(NEVPNError(.connectionFailed))
                return
            }
            self.logger.info("vpn", "Tunnel established", fields: ["fd": Int(tunFd)])

            do {
                try MihomobridgePatchConfigWithTun(configPath, Int(tunFd))
                self.logger.info("vpn", "Patched config.yaml with TUN fd",
                                 fields: ["file-descriptor": Int(tunFd), "stack": "system"])
                try MihomobridgeStart(home, configPath, self.coreLogAdapter)
                self.logger.info("mihomo", "Core started in-process")
                completionHandler(nil)
            } catch {
                self.logger.error("mihomo", "Core start failed",
                                  fields: ["error": error.localizedDescription])
                completionHandler(error)
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason,
                             completionHandler: @escaping () -> Void) {
        logger.info("vpn", "Stopping tunnel", fields: ["reason": String(describing: reason)])
        MihomobridgeStop()
        completionHandler()
    }

    /// NEPacketTunnelProvider exposes packetFlow, not the fd, but mihomo's
    /// sing-tun wants a real descriptor.  Apple creates the utun inside this
    /// process, so scan our fd table for the utun control socket — the
    /// technique WireGuardKit and sing-box use.
    private func tunnelFileDescriptor() -> Int32? {
        var ctlInfo = ctl_info()
        withUnsafeMutablePointer(to: &ctlInfo.ctl_name) {
            $0.withMemoryRebound(to: CChar.self,
                                 capacity: MemoryLayout.size(ofValue: $0.pointee)) {
                _ = strcpy($0, "com.apple.net.utun_control")
            }
        }
        for fd: Int32 in 0...1024 {
            var addr = sockaddr_ctl()
            var len = socklen_t(MemoryLayout.size(ofValue: addr))
            let ret = withUnsafeMutablePointer(to: &addr) {
                $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                    getpeername(fd, $0, &len)
                }
            }
            if ret != 0 || addr.sc_family != AF_SYSTEM {
                continue
            }
            if ctlInfo.ctl_id == 0 {
                if ioctl(fd, CTLIOCGINFO, &ctlInfo) != 0 {
                    continue
                }
            }
            if addr.sc_id == ctlInfo.ctl_id {
                return fd
            }
        }
        return nil
    }
}

/// Forwards mihomo core log lines into the shared JSONL file the app tails.
private final class CoreLogAdapter: NSObject, MihomobridgeLogCallbackProtocol {
    private let logger: ExtensionLogger

    init(logger: ExtensionLogger) {
        self.logger = logger
    }

    func onLog(_ level: String?, payload: String?) {
        guard let payload = payload, !payload.isEmpty else { return }
        // mihomo levels: debug/info/warning/error/silent → Android schema
        let normalized: String
        switch level ?? "info" {
        case "warning": normalized = "warn"
        case "silent": normalized = "debug"
        default: normalized = level ?? "info"
        }
        logger.emit(level: normalized, component: "mihomo", message: payload)
    }
}
