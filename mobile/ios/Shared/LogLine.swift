import Foundation

/// Builds the JSON log payload consumed by Dart's `_onNativeLog`.
/// Schema mirrors Android's LogEventBridge exactly:
/// {"level","component","message","ts",("fields")}.
/// Compiled into BOTH targets.
enum LogLine {
    static func encode(level: String, component: String, message: String,
                       fields: [String: Any] = [:]) -> String {
        var obj: [String: Any] = [
            "level": level,
            "component": component,
            "message": message,
            "ts": Int(Date().timeIntervalSince1970 * 1000),
        ]
        if !fields.isEmpty {
            obj["fields"] = fields
        }
        guard JSONSerialization.isValidJSONObject(obj),
              let data = try? JSONSerialization.data(withJSONObject: obj),
              let json = String(data: data, encoding: .utf8) else {
            return #"{"level":"\#(level)","component":"\#(component)","message":"log-encode-failed"}"#
        }
        return json
    }
}
