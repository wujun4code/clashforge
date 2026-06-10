import Foundation

/// Append-only JSONL logger for the PacketTunnel process.
///
/// The extension cannot reach the app's EventChannel directly (separate
/// process), so it writes LogLine-encoded JSON to
/// <app-group>/logs/extension.jsonl; the app's LogEventBridge tails the file
/// and replays each line into Dart unchanged.
final class ExtensionLogger {
    /// Truncate once the file outgrows this; the app-side tailer detects the
    /// shrink and resets its offset.
    private static let maxFileSize: UInt64 = 2 * 1024 * 1024

    private let queue = DispatchQueue(label: "com.clashforge.extension-logger")

    func emit(level: String, component: String, message: String,
              fields: [String: Any] = [:]) {
        let line = LogLine.encode(level: level, component: component,
                                  message: message, fields: fields)
        queue.async {
            self.append(line)
        }
        NSLog("[%@] %@: %@", level, component, message)
    }

    func debug(_ component: String, _ message: String, fields: [String: Any] = [:]) {
        emit(level: "debug", component: component, message: message, fields: fields)
    }

    func info(_ component: String, _ message: String, fields: [String: Any] = [:]) {
        emit(level: "info", component: component, message: message, fields: fields)
    }

    func warn(_ component: String, _ message: String, fields: [String: Any] = [:]) {
        emit(level: "warn", component: component, message: message, fields: fields)
    }

    func error(_ component: String, _ message: String, fields: [String: Any] = [:]) {
        emit(level: "error", component: component, message: message, fields: fields)
    }

    private func append(_ line: String) {
        let fm = FileManager.default
        let url = SharedPaths.extensionLogURL
        try? fm.createDirectory(at: SharedPaths.logDirectoryURL,
                                withIntermediateDirectories: true)

        if let attrs = try? fm.attributesOfItem(atPath: url.path),
           let size = attrs[.size] as? UInt64, size > Self.maxFileSize {
            try? fm.removeItem(at: url)
        }
        if !fm.fileExists(atPath: url.path) {
            fm.createFile(atPath: url.path, contents: nil)
        }

        guard let handle = try? FileHandle(forWritingTo: url),
              let data = (line + "\n").data(using: .utf8) else {
            return
        }
        defer { try? handle.close() }
        _ = try? handle.seekToEnd()
        try? handle.write(contentsOf: data)
    }
}
