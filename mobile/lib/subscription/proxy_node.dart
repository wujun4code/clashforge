import 'dart:convert';

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
    return ProxyNode(
      name: json['name'] as String? ?? 'Unknown',
      type: json['type'] as String? ?? 'unknown',
      server: json['server'] as String? ?? '',
      port: json['port'] as int? ?? 0,
      raw: json,
    );
  }

  Map<String, dynamic> toJson() => raw;
}
