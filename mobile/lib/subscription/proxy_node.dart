class ProxyNode {
  final String name;
  final String type;
  final String server;
  final int port;
  final Map<String, dynamic> raw;

  ProxyNode({
    required this.name,
    required this.type,
    required this.server,
    required this.port,
    required this.raw,
  });

  factory ProxyNode.fromJson(Map<String, dynamic> json) {
    final parsedPort = _coercePort(json['port']);
    final raw = Map<String, dynamic>.from(json);
    raw['port'] = parsedPort;
    return ProxyNode(
      name: _coerceString(json['name'], fallback: 'Unknown'),
      type: _coerceString(json['type'], fallback: 'unknown'),
      server: _coerceString(json['server']),
      port: parsedPort,
      raw: raw,
    );
  }

  Map<String, dynamic> toJson() => raw;

  static String _coerceString(dynamic value, {String fallback = ''}) {
    if (value == null) return fallback;
    final s = value.toString().trim();
    return s.isEmpty ? fallback : s;
  }

  static int _coercePort(dynamic value) {
    if (value is int) return value;
    if (value is num) return value.toInt();
    if (value == null) return 0;
    final s = value.toString().trim();
    if (s.isEmpty) return 0;
    return int.tryParse(s) ?? 0;
  }
}
