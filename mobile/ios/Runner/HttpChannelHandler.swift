import Flutter
import Foundation

/// MethodChannel "com.clashforge.mobile/http" — native fetch for subscription
/// downloads.  Android uses Cronet for a Chrome JA3 fingerprint; on iOS,
/// URLSession negotiates Apple's TLS stack (Safari fingerprint), which the
/// same JA3-picky subscription servers also accept.  Dart falls back to its
/// own HTTP client if this channel errors.
final class HttpChannelHandler: NSObject {
    static let channelName = "com.clashforge.mobile/http"

    private static let browserHeaders: [String: String] = [
        "User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) "
            + "AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,"
            + "image/avif,image/webp,image/apng,*/*;q=0.8,"
            + "application/signed-exchange;v=b3;q=0.7",
        "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        "Cache-Control": "no-cache",
    ]

    static func register(with messenger: FlutterBinaryMessenger) {
        let channel = FlutterMethodChannel(name: channelName, binaryMessenger: messenger)
        channel.setMethodCallHandler { call, result in
            switch call.method {
            case "fetchUrl":
                fetchUrl(call, result: result)
            default:
                result(FlutterMethodNotImplemented)
            }
        }
    }

    private static func fetchUrl(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        let args = call.arguments as? [String: Any]
        guard let urlString = args?["url"] as? String, let url = URL(string: urlString) else {
            result(FlutterError(code: "INVALID_ARG", message: "url is required", details: nil))
            return
        }
        let timeoutMs = min(max(args?["timeoutMs"] as? Int ?? 15000, 3000), 60000)

        var request = URLRequest(url: url)
        request.timeoutInterval = Double(timeoutMs) / 1000.0
        request.cachePolicy = .reloadIgnoringLocalCacheData
        for (k, v) in browserHeaders {
            request.setValue(v, forHTTPHeaderField: k)
        }

        URLSession.shared.dataTask(with: request) { data, response, error in
            DispatchQueue.main.async {
                if let error = error {
                    result(FlutterError(code: "FETCH_ERROR",
                                        message: error.localizedDescription, details: nil))
                    return
                }
                let status = (response as? HTTPURLResponse)?.statusCode ?? 0
                let body = data.flatMap { String(data: $0, encoding: .utf8) } ?? ""
                result(["status": status, "body": body])
            }
        }.resume()
    }
}
