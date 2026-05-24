import 'dart:convert';
import 'package:yaml/yaml.dart';
import 'parsed_subscription.dart';
import 'proxy_node.dart';

class SubscriptionParser {
  static ParsedSubscription parse(String content) {
    content = sanitizeInput(content);
    final trimmed = content.trim();

    // 1. Try Clash YAML with "proxies" key — also extract rules / proxy-groups.
    if (trimmed.contains('proxies:')) {
      try {
        final doc = loadYaml(trimmed);
        if (doc is Map &&
            doc.containsKey('proxies') &&
            doc['proxies'] is List) {
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

          if (proxies.isNotEmpty ||
              proxyGroups.isNotEmpty ||
              rules.isNotEmpty ||
              ruleProviders.isNotEmpty) {
            return ParsedSubscription(
              proxies: proxies,
              proxyGroups: proxyGroups,
              rules: rules,
              ruleProviders: ruleProviders,
            );
          }
        }
      } catch (_) {}
    }

    // 2. Try Bare YAML sequence
    if (trimmed.startsWith('- ') || trimmed.startsWith('-\n')) {
      try {
        final doc = loadYaml(trimmed);
        if (doc is List) {
          final proxies = _parseYamlList(doc);
          if (proxies.isNotEmpty) {
            return ParsedSubscription(proxies: proxies);
          }
        }
      } catch (_) {}
    }

    // 2b. Try a single YAML map node (users often paste one node block).
    try {
      final doc = loadYaml(trimmed);
      if (doc is Map && _looksLikeSingleProxyMap(doc)) {
        final normalized = _normalizeProxyMap(_convertYamlMap(doc));
        return ParsedSubscription(
          proxies: [ProxyNode.fromJson(normalized)],
        );
      }
    } catch (_) {}

    // 2c. Try parsing key/value lines as a single proxy block (clipboard fallback).
    final kvProxy = _parseSingleProxyFromKeyValueLines(trimmed);
    if (kvProxy != null) {
      return ParsedSubscription(proxies: [kvProxy]);
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

  static String sanitizeInput(String content) {
    var text = _normalizePastedYaml(content);
    text = _stripCommonIndent(text);

    final compacted = <String>[];
    var previousBlank = false;
    for (final rawLine in LineSplitter.split(text)) {
      final line = rawLine.replaceAll(RegExp(r'[ \t]+$'), '');
      final isBlank = line.trim().isEmpty;
      if (isBlank) {
        if (previousBlank) continue;
        compacted.add('');
        previousBlank = true;
        continue;
      }
      compacted.add(line);
      previousBlank = false;
    }

    while (compacted.isNotEmpty && compacted.first.trim().isEmpty) {
      compacted.removeAt(0);
    }
    while (compacted.isNotEmpty && compacted.last.trim().isEmpty) {
      compacted.removeLast();
    }
    return compacted.join('\n');
  }

  static List<ProxyNode> _parseYamlList(List list) {
    final nodes = <ProxyNode>[];
    for (final item in list) {
      if (item is Map) {
        try {
          nodes.add(
              ProxyNode.fromJson(_normalizeProxyMap(_convertYamlMap(item))));
        } catch (_) {
          // Skip malformed entries and continue parsing remaining nodes.
        }
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
      final name =
          parts.length > 1 ? Uri.decodeComponent(parts[1]) : 'Shadowsocks';
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
          raw: {
            'name': name,
            'type': 'ss',
            'server': serverParts[0],
            'port': int.parse(serverParts[1])
          },
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
      final name = parsed.fragment.isNotEmpty
          ? Uri.decodeComponent(parsed.fragment)
          : 'Trojan';
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
      final name = parsed.fragment.isNotEmpty
          ? Uri.decodeComponent(parsed.fragment)
          : 'VLESS';
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

  static bool _looksLikeSingleProxyMap(Map map) {
    final normalizedKeys =
        map.keys.map((e) => e.toString().trim().toLowerCase()).toSet();
    final hasType = normalizedKeys.contains('type');
    final hasServer = normalizedKeys.contains('server');
    // "name" is optional in fallback mode; we'll synthesize one from server.
    return hasType && hasServer;
  }

  // Make pasted snippets more forgiving:
  // - strip markdown code fences (```yaml ... ```)
  // - normalize line endings
  // - turn leading TAB indentation into spaces
  // - remove invisible clipboard chars (BOM / zero-width chars)
  static String _normalizePastedYaml(String content) {
    var text = content.replaceAll('\r\n', '\n').replaceAll('\r', '\n');
    text = text
        .replaceAll('\uFEFF', '')
        .replaceAll('\u200B', '')
        .replaceAll('\u200C', '')
        .replaceAll('\u200D', '')
        .replaceAll('\u2060', '')
        .replaceAll('\u2028', '\n')
        .replaceAll('\u2029', '\n')
        .replaceAll('：', ':')
        .replaceAll('，', ',');

    final lines = LineSplitter.split(text).toList();
    if (lines.length >= 2) {
      final first = lines.first.trim();
      final last = lines.last.trim();
      final isBacktickFence = first.startsWith('```') && last.startsWith('```');
      final isTildeFence = first.startsWith('~~~') && last.startsWith('~~~');
      if (isBacktickFence || isTildeFence) {
        text = lines.sublist(1, lines.length - 1).join('\n');
      }
    }

    final normalized = <String>[];
    for (final line in LineSplitter.split(text)) {
      var replaced = line.replaceAll('\u00A0', ' ');
      // Normalize common non-ASCII list bullets to YAML list marker "- ".
      replaced = replaced.replaceFirstMapped(
        RegExp(r'^([\t ]*)[–—−•·●▪◦]+\s+'),
        (m) => '${m.group(1) ?? ''}- ',
      );
      final indent = RegExp(r'^[\t ]+').firstMatch(replaced)?.group(0);
      if (indent == null || indent.isEmpty) {
        normalized.add(replaced);
        continue;
      }
      final fixedIndent = indent.replaceAll('\t', '  ');
      normalized.add('$fixedIndent${replaced.substring(indent.length)}');
    }
    return normalized.join('\n');
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
    return lines
        .map((line) =>
            line.length >= minIndent! ? line.substring(minIndent) : '')
        .join('\n');
  }

  static ProxyNode? _parseSingleProxyFromKeyValueLines(String content) {
    if (content.isEmpty) return null;
    final map = <String, dynamic>{};

    for (final rawLine in LineSplitter.split(content)) {
      var line = rawLine.trim();
      if (line.isEmpty || line.startsWith('#')) continue;
      line = line.replaceFirst(
        RegExp(r'^[\-\u2013\u2014\u2212\u2022\u00B7\u25CF\u25AA\u25E6]+\s*'),
        '',
      );
      final m = RegExp(r'^([A-Za-z0-9_-]+)\s*:\s*(.*)$').firstMatch(line);
      if (m == null) continue;
      final key = m.group(1)!.trim().toLowerCase();
      final rawValue = m.group(2)!.trim();
      map[key] = _parseLooseScalar(rawValue);
    }

    if (!_looksLikeSingleProxyMap(map)) return null;

    final server = (map['server'] ?? '').toString().trim();
    final type = (map['type'] ?? '').toString().trim();
    if (server.isEmpty || type.isEmpty) return null;
    map['server'] = server;
    map['type'] = type;

    final name = (map['name'] ?? '').toString().trim();
    if (name.isEmpty) {
      map['name'] = '$type-$server';
    }

    if (!map.containsKey('port')) {
      map['port'] = _defaultPortByType(type);
    }

    try {
      return ProxyNode.fromJson(_normalizeProxyMap(map));
    } catch (_) {
      return null;
    }
  }

  static dynamic _parseLooseScalar(String raw) {
    var value = raw.trim();
    if (value.isEmpty) return '';

    if (value.length >= 2 &&
        ((value.startsWith('"') && value.endsWith('"')) ||
            (value.startsWith('\'') && value.endsWith('\'')))) {
      value = value.substring(1, value.length - 1).trim();
    }

    final lower = value.toLowerCase();
    if (lower == 'true') return true;
    if (lower == 'false') return false;

    final intValue = int.tryParse(value);
    if (intValue != null) return intValue;

    final doubleValue = double.tryParse(value);
    if (doubleValue != null) return doubleValue;

    return value;
  }

  static int _defaultPortByType(String type) {
    switch (type.toLowerCase()) {
      case 'http':
      case 'https':
      case 'trojan':
      case 'vmess':
      case 'vless':
      case 'ss':
      case 'hysteria':
      case 'hysteria2':
        return 443;
      default:
        return 0;
    }
  }

  static Map<String, dynamic> _normalizeProxyMap(Map<String, dynamic> map) {
    final normalized = <String, dynamic>{};
    map.forEach((k, v) {
      normalized[k.toString().trim().toLowerCase()] = v;
    });

    final server = (normalized['server'] ?? '').toString().trim();
    final type = (normalized['type'] ?? '').toString().trim();
    if (server.isNotEmpty) normalized['server'] = server;
    if (type.isNotEmpty) normalized['type'] = type;

    final name = (normalized['name'] ?? '').toString().trim();
    if (name.isEmpty && server.isNotEmpty) {
      normalized['name'] = type.isEmpty ? server : '$type-$server';
    }

    final currentPort = int.tryParse((normalized['port'] ?? '').toString());
    if ((currentPort == null || currentPort <= 0) && type.isNotEmpty) {
      normalized['port'] = _defaultPortByType(type);
    }

    return normalized;
  }
}
