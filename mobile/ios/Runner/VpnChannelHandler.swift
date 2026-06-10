import Flutter
import Foundation
import NetworkExtension

/// MethodChannel "com.clashforge.mobile/vpn" — same five methods as Android's
/// MainActivity so the Dart VpnManager works unchanged.
///
/// Android model: VpnService in the same process, started via Intent.
/// iOS model: NETunnelProviderManager preference + PacketTunnel extension in
/// a separate process.  "permission_needed" maps to the user declining the
/// one-time "Allow ClashForge to add VPN configurations" system dialog
/// (Android: VpnService.prepare consent activity).
final class VpnChannelHandler: NSObject {
    static let channelName = "com.clashforge.mobile/vpn"

    static func register(with messenger: FlutterBinaryMessenger) {
        let channel = FlutterMethodChannel(name: channelName, binaryMessenger: messenger)
        let handler = VpnChannelHandler()
        channel.setMethodCallHandler { call, result in
            handler.handle(call, result: result)
        }
        // Retain the handler for the lifetime of the channel.
        objc_setAssociatedObject(channel, &handlerKey, handler, .OBJC_ASSOCIATION_RETAIN)
    }

    private func handle(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        switch call.method {
        case "startVpn":
            startVpn(result: result)
        case "stopVpn":
            stopVpn(result: result)
        case "getFilesDir":
            result(SharedPaths.containerURL.path)
        case "writeConfig":
            writeConfig(call, result: result)
        case "getSystemInfo":
            buildSystemInfo(result: result)
        default:
            result(FlutterMethodNotImplemented)
        }
    }

    // MARK: - start / stop

    private func startVpn(result: @escaping FlutterResult) {
        extractGeodataAssetsIfNeeded()

        NETunnelProviderManager.loadAllFromPreferences { managers, loadError in
            if let loadError = loadError {
                result(FlutterError(code: "LOAD_FAILED",
                                    message: loadError.localizedDescription, details: nil))
                return
            }

            let manager = managers?.first ?? NETunnelProviderManager()
            let proto = (manager.protocolConfiguration as? NETunnelProviderProtocol)
                ?? NETunnelProviderProtocol()
            proto.providerBundleIdentifier = SharedPaths.tunnelBundleID
            proto.serverAddress = "ClashForge"
            manager.protocolConfiguration = proto
            manager.localizedDescription = "ClashForge 畅行"
            manager.isEnabled = true

            manager.saveToPreferences { saveError in
                if let saveError = saveError as NSError? {
                    // First save triggers the system VPN-configuration consent
                    // dialog; declining surfaces as configurationReadWriteFailed.
                    if saveError.domain == NEVPNErrorDomain,
                       saveError.code == NEVPNError.configurationReadWriteFailed.rawValue {
                        result("permission_needed")
                    } else {
                        result(FlutterError(code: "SAVE_FAILED",
                                            message: saveError.localizedDescription, details: nil))
                    }
                    return
                }
                // Reload after save — starting from a stale manager object
                // fails with "configuration is invalid" (long-standing NE quirk).
                manager.loadFromPreferences { reloadError in
                    if let reloadError = reloadError {
                        result(FlutterError(code: "LOAD_FAILED",
                                            message: reloadError.localizedDescription, details: nil))
                        return
                    }
                    do {
                        try manager.connection.startVPNTunnel()
                        LogEventBridge.shared.info("vpn", "Tunnel start requested")
                        result("started")
                    } catch {
                        result(FlutterError(code: "START_FAILED",
                                            message: error.localizedDescription, details: nil))
                    }
                }
            }
        }
    }

    private func stopVpn(result: @escaping FlutterResult) {
        NETunnelProviderManager.loadAllFromPreferences { managers, _ in
            managers?.first?.connection.stopVPNTunnel()
            LogEventBridge.shared.info("vpn", "Tunnel stop requested")
            result("stopped")
        }
    }

    // MARK: - config

    private func writeConfig(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        let args = call.arguments as? [String: Any]
        let yaml = args?["yaml"] as? String ?? ""
        do {
            try FileManager.default.createDirectory(at: SharedPaths.containerURL,
                                                    withIntermediateDirectories: true)
            try yaml.write(to: SharedPaths.configURL, atomically: true, encoding: .utf8)
            result("written")
        } catch {
            result(FlutterError(code: "WRITE_FAILED",
                                message: error.localizedDescription, details: nil))
        }
    }

    // MARK: - geodata assets

    /// Android extracts geosite.dat / country.mmdb from APK assets in the
    /// VpnService; on iOS the extension must stay lean (50 MB jetsam limit),
    /// so the app copies them into the App Group container instead.
    private func extractGeodataAssetsIfNeeded() {
        let fm = FileManager.default
        let container = SharedPaths.containerURL
        let marker = container.appendingPathComponent(".asset_version")
        let currentVersion = Bundle.main.object(
            forInfoDictionaryKey: "CFBundleVersion") as? String ?? "0"

        if let existing = try? String(contentsOf: marker, encoding: .utf8),
           existing.trimmingCharacters(in: .whitespacesAndNewlines) == currentVersion {
            return
        }

        let flutterAssets = Bundle.main.bundleURL
            .appendingPathComponent("Frameworks/App.framework/flutter_assets/assets/geodata")
        for name in ["country.mmdb", "geosite.dat"] {
            let src = flutterAssets.appendingPathComponent(name)
            let dst = container.appendingPathComponent(name)
            do {
                if fm.fileExists(atPath: dst.path) {
                    try fm.removeItem(at: dst)
                }
                try fm.copyItem(at: src, to: dst)
                LogEventBridge.shared.debug("assets", "Extracted \(name)")
            } catch {
                LogEventBridge.shared.error("assets", "Extract failed: \(name)",
                                            fields: ["error": error.localizedDescription])
            }
        }
        try? currentVersion.write(to: marker, atomically: true, encoding: .utf8)
        LogEventBridge.shared.info("assets", "Extraction complete",
                                   fields: ["version": currentVersion])
    }

    // MARK: - system info

    private func buildSystemInfo(result: @escaping FlutterResult) {
        NETunnelProviderManager.loadAllFromPreferences { managers, _ in
            let status = managers?.first?.connection.status ?? .invalid
            let vpnRunning = status == .connected || status == .connecting
                || status == .reasserting
            let info: [String: Any] = [
                "app_version": Bundle.main.object(
                    forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "unknown",
                "build_number": Int(Bundle.main.object(
                    forInfoDictionaryKey: "CFBundleVersion") as? String ?? "0") ?? 0,
                "vpn_running": vpnRunning,
                // mihomo lives inside the extension process; connected tunnel
                // implies a running core.  No PID across process boundaries.
                "mihomo_running": status == .connected,
                "mihomo_pid": -1,
                "memory_app_pss_mb": Self.appMemoryFootprintMB(),
                "memory_available_mb": Double(ProcessInfo.processInfo.physicalMemory)
                    / 1024.0 / 1024.0,
                "device_abi": "arm64",
                // Android-only concepts; Dart treats empty string as "off".
                "private_dns_mode": "",
                "private_dns_specifier": "",
            ]
            result(info)
        }
    }

    private static func appMemoryFootprintMB() -> Double {
        var info = task_vm_info_data_t()
        var count = TASK_VM_INFO_COUNT
        let kr = withUnsafeMutablePointer(to: &info) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count)
            }
        }
        guard kr == KERN_SUCCESS else { return 0 }
        return Double(info.phys_footprint) / 1024.0 / 1024.0
    }
}

private var handlerKey: UInt8 = 0

private let TASK_VM_INFO_COUNT = mach_msg_type_number_t(
    MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<integer_t>.size)
