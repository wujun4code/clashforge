import Foundation

/// Paths and identifiers shared between the Runner app and the PacketTunnel
/// extension.  This file is compiled into BOTH targets.
///
/// Everything the two processes exchange lives in the App Group container:
/// config.yaml (written by the app, read by mihomo), geodata
/// (geosite.dat / country.mmdb, extracted by the app from flutter assets)
/// and logs/extension.jsonl (written by the extension, tailed by the app's
/// EventChannel bridge).
enum SharedPaths {
    /// Must match the application-groups entry in both .entitlements files.
    static let appGroupID = "group.com.clashforge.clashforgeMobile"

    /// Bundle id of the packet tunnel extension (providerBundleIdentifier).
    static let tunnelBundleID = "com.clashforge.clashforgeMobile.PacketTunnel"

    /// App Group container root — the iOS equivalent of Android's filesDir.
    /// Falls back to the app's own Documents directory when the entitlement
    /// is missing (unsigned CI builds, unit tests) so nothing force-unwraps.
    static var containerURL: URL {
        if let url = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: appGroupID) {
            return url
        }
        return FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
    }

    static var configURL: URL {
        containerURL.appendingPathComponent("config.yaml")
    }

    static var logDirectoryURL: URL {
        containerURL.appendingPathComponent("logs", isDirectory: true)
    }

    static var extensionLogURL: URL {
        logDirectoryURL.appendingPathComponent("extension.jsonl")
    }
}
