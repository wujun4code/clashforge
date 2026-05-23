import 'dart:convert';
import 'package:yaml/yaml.dart';
import 'parsed_subscription.dart';
import 'proxy_node.dart';

class SubscriptionParser {
  static ParsedSubscription parse(String content) {
    content = _stripCommonIndent(content);
    final trimmed = content.trim();

    // 1. Try Clash YAML with "proxies" key — also extract rules / proxy-groups.
    if (trimmed.contains('proxies:')) {
      try {
        final doc = loadYaml(trimmed);
        if (doc is Map && doc.containsKey('proxies') && doc['proxies'] is List) {
          final proxies = _parseYamlList(doc['proxies'] as List);

          // Extract proxy-groups (pass-through raw maps).
          final rawGroups = doc['proxy-groups'];
          final proxyGroups = <Map<String, dynamic>>[];
          if (rawGroups is List) {
            for (final g in rawGroups) {
              if (g is Map) {
                proxyGroups.add(_convertYamlMap(g));
              }
            }
          }

          // Extract rules — presence signals "custom rules" mode.
          // DNS is intentionally ignored (spec item 3).
          final rawRules = doc['rules'];
          final rules = <String>[];
          if (rawRules is List) {
            for (final r in rawRules) {
              final s = r?.toString().trim() ?? '';
              if (s.isNotEmpty) rules.add(s);
            }
          }

          // Extract rule-providers so mihomo can download referenced rule sets.
          final rawProviders = doc['rule-providers'];
          final ruleProviders = <String, Map<String, dynamic>>{};
          if (rawProviders is Map) {
            rawProviders.forEach((k, v) {
              if (v is Map) ruleProviders[k.toString()] = _convertYamlMap(v);
            });
          }

          return ParsedSubscription(
            proxies: proxies,
            proxyGroups: proxyGroups,
            rules: rules,
            ruleProviders: ruleProviders,
          );
        }
      } catch (_) {}
    }

    // 2. Try Bare YAML sequence
    if (trimmed.startsWith('- ') || trimmed.startsWith('-\n')) {
      try {
        final doc = loadYaml(trimmed);
        if (doc is List) {
          return ParsedSubscription(proxies: _parseYamlList(doc));
        }
      } catch (_) {}
    }

    // 3. Try Base64 decoding
    if (_looksLikeBase64(trimmed)) {
      try {
        final decoded = utf8.decode(base64.decode(base64.normalize(trimmed)));
        return ParsedSubscription(proxies: _parseLineBased(decoded));
      } catch (_) {}
    }

    // 4. Line-based URIs
    return ParsedSubscription(proxies: _parseLineBased(trimmed));
  }

  static List<ProxyNode> _parseYamlList(List list) {
    final nodes = <ProxyNode>[];
    for (final item in list) {
      if (item is Map) {
        nodes.add(ProxyNode.fromJson(_convertYamlMap(item)));
      }
    }
    return nodes;
  }

  static Map<String, dynamic> _convertYamlMap(Map m) {
    final out = <String, dynamic>{};
    m.forEach((k, v) => out[k.toString()] = _convertYamlValue(v));
    return out;
  }

  static dynamic _convertYamlValue(dynamic val) {
    if (val is YamlMap) return _convertYamlMap(val);
    if (val is YamlList) return val.map(_convertYamlValue).toList();
    return val;
  }

  static List<ProxyNode> _parseLineBased(String content) {
    final nodes = <ProxyNode>[];
    final lines = LineSplitter.split(content);
    for (final line in lines) {
      final trimmed = line.trim();
      if (trimmed.isEmpty || trimmed.startsWith('#')) continue;

      ProxyNode? node;
      if (trimmed.startsWith('ss://')) {
        node = _parseSS(trimmed);
      } else if (trimmed.startsWith('vmess://')) {
        node = _parseVmess(trimmed);
      } else if (trimmed.startsWith('trojan://')) {
        node = _parseTrojan(trimmed);
      } else if (trimmed.startsWith('vless://')) {
        node = _parseVless(trimmed);
      }

      if (node != null) {
        nodes.add(node);
      }
    }
    return nodes;
  }

  static ProxyNode? _parseSS(String uri) {
    try {
      final cleanUri = uri.substring(5);
      final parts = cleanUri.split('#');
      final name = parts.length > 1 ? Uri.decodeComponent(parts[1]) : 'Shadowsocks';
      final mainPart = parts[0];

      if (mainPart.contains('@')) {
        final atParts = mainPart.split('@');
        final serverPort = atParts[1];
        final serverParts = serverPort.split(':');
        return ProxyNode(
          name: name,
          type: 'ss',
          server: serverParts[0],
          port: int.parse(serverParts[1]),
          raw: {'name': name, 'type': 'ss', 'server': serverParts[0], 'port': int.parse(serverParts[1])},
        );
      }
    } catch (_) {}
    return null;
  }

  static ProxyNode? _parseVmess(String uri) {
    try {
      final cleanUri = uri.substring(8);
      final decoded = utf8.decode(base64.decode(base64.normalize(cleanUri)));
      final map = json.decode(decoded) as Map<String, dynamic>;
      final name = map['ps'] ?? 'VMess';
      return ProxyNode(
        name: name,
        type: 'vmess',
        server: map['add'] ?? '',
        port: int.tryParse(map['port']?.toString() ?? '443') ?? 443,
        raw: {
          'name': name,
          'type': 'vmess',
          'server': map['add'] ?? '',
          'port': int.tryParse(map['port']?.toString() ?? '443') ?? 443,
          'uuid': map['id'],
          'alterId': int.tryParse(map['aid']?.toString() ?? '0') ?? 0,
          'cipher': 'auto',
        },
      );
    } catch (_) {}
    return null;
  }

  static ProxyNode? _parseTrojan(String uri) {
    try {
      final parsed = Uri.parse(uri);
      final name = parsed.fragment.isNotEmpty ? Uri.decodeComponent(parsed.fragment) : 'Trojan';
      return ProxyNode(
        name: name,
        type: 'trojan',
        server: parsed.host,
        port: parsed.port,
        raw: {
          'name': name,
          'type': 'trojan',
          'server': parsed.host,
          'port': parsed.port,
          'password': parsed.userInfo,
          'sni': parsed.queryParameters['sni'],
        },
      );
    } catch (_) {}
    return null;
  }

  static ProxyNode? _parseVless(String uri) {
    try {
      final parsed = Uri.parse(uri);
      final name = parsed.fragment.isNotEmpty ? Uri.decodeComponent(parsed.fragment) : 'VLESS';
      return ProxyNode(
        name: name,
        type: 'vless',
        server: parsed.host,
        port: parsed.port,
        raw: {
          'name': name,
          'type': 'vless',
          'server': parsed.host,
          'port': parsed.port,
          'uuid': parsed.userInfo,
          'tls': parsed.queryParameters['security'] == 'tls',
          'sni': parsed.queryParameters['sni'],
        },
      );
    } catch (_) {}
    return null;
  }

  static bool _looksLikeBase64(String s) {
    if (s.length < 8 || s.contains('://') || s.contains(' ')) return false;
    final validChars = RegExp(r'^[A-Za-z0-9+/=\-_]+$');
    return validChars.hasMatch(s);
  }

  static String _stripCommonIndent(String content) {
    final lines = LineSplitter.split(content).toList();
    if (lines.isEmpty) return content;
    int? minIndent;
    for (final line in lines) {
      if (line.trim().isEmpty) continue;
      final indent = line.length - line.trimLeft().length;
      if (minIndent == null || indent < minIndent) {
        minIndent = indent;
      }
    }
    if (minIndent == null || minIndent == 0) return content;
    return lines.map((line) => line.length >= minIndent! ? line.substring(minIndent) : '').join('\n');
  }
}
