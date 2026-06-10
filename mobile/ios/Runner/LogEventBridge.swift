import Flutter
import Foundation

/// EventChannel "com.clashforge.mobile/logs" — streams JSON log strings to
/// Dart in the schema Android's LogEventBridge uses.
///
/// Two sources feed the stream:
///   1. App-side events emitted directly via info()/warn()/error()/debug().
///   2. The PacketTunnel extension, which runs in a separate process and
///      appends JSONL to logs/extension.jsonl in the App Group container.
///      A 500 ms poller tails that file from the last read offset (Android
///      needs no transport here because its service shares the process).
final class LogEventBridge: NSObject, FlutterStreamHandler {
    static let channelName = "com.clashforge.mobile/logs"
    static let shared = LogEventBridge()

    private var sink: FlutterEventSink?
    private var timer: Timer?
    private var fileOffset: UInt64 = 0

    static func register(with messenger: FlutterBinaryMessenger) {
        let channel = FlutterEventChannel(name: channelName, binaryMessenger: messenger)
        channel.setStreamHandler(shared)
    }

    // MARK: - FlutterStreamHandler

    func onListen(withArguments arguments: Any?,
                  eventSink events: @escaping FlutterEventSink) -> FlutterError? {
        sink = events
        // Tail from the current end: history before this app session belongs
        // to previous runs and would flood the Dart log buffer.
        fileOffset = currentLogFileSize()
        timer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { [weak self] _ in
            self?.drainExtensionLog()
        }
        return nil
    }

    func onCancel(withArguments arguments: Any?) -> FlutterError? {
        timer?.invalidate()
        timer = nil
        sink = nil
        return nil
    }

    // MARK: - app-side emit (Kotlin LogEventBridge API mirror)

    func emit(level: String, component: String, message: String, fields: [String: Any] = [:]) {
        let payload = LogLine.encode(level: level, component: component,
                                     message: message, fields: fields)
        DispatchQueue.main.async { [weak self] in
            self?.sink?(payload)
        }
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

    // MARK: - extension log tailing

    private func currentLogFileSize() -> UInt64 {
        let attrs = try? FileManager.default.attributesOfItem(
            atPath: SharedPaths.extensionLogURL.path)
        return attrs?[.size] as? UInt64 ?? 0
    }

    private func drainExtensionLog() {
        guard let sink = sink else { return }
        guard let handle = try? FileHandle(forReadingFrom: SharedPaths.extensionLogURL) else {
            return
        }
        defer { try? handle.close() }

        let size = (try? handle.seekToEnd()) ?? 0
        if size < fileOffset {
            // Extension truncated/rotated the file — restart from the top.
            fileOffset = 0
        }
        guard size > fileOffset else { return }

        try? handle.seek(toOffset: fileOffset)
        guard let data = try? handle.readToEnd(), !data.isEmpty else { return }

        // Only consume complete lines; a partially-flushed tail line is
        // picked up on the next tick.
        var consumable = data
        if data.last != UInt8(ascii: "\n") {
            guard let lastNewline = data.lastIndex(of: UInt8(ascii: "\n")) else { return }
            consumable = data.prefix(through: lastNewline)
        }
        fileOffset += UInt64(consumable.count)

        guard let text = String(data: consumable, encoding: .utf8) else { return }
        for line in text.split(separator: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if !trimmed.isEmpty {
                sink(trimmed)
            }
        }
    }
}
