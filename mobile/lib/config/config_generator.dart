import 'dart:io';
import 'loyalsoldier_template.dart';
import '../subscription/proxy_node.dart';

class ConfigGenerator {
  static const _cnPlainDns = ['223.5.5.5', '119.29.29.29'];
  static const _cnDoh = [
    'https://dns.alidns.com/dns-query',
    'https://doh.pub/dns-query',
  ];
  static const _intlDoh = [
    'https://1.1.1.1/dns-query',
    'https://8.8.8.8/dns-query',
  ];
  static const _fakeIpFilter = [
    '*.lan',
    '*.local',
    '*.localhost',
    '*.localdomain',
    '+.stun.*.*',
    '+.stun.*.*.*',
    'msftconnecttest.com',
    '*.msftconnecttest.com',
    'time.*.com',
    'ntp.*.com',
    '*.ntp.org.cn',
    '*.pool.ntp.org',
    // Tencent / WeChat — video upload uses real-IP CDN nodes selected by
    // WeChat's own HTTP-DNS; fake-ip would route QUIC to an unreachable fake
    // address and trigger upload retry storms.
    '+.qq.com',
    '+.qpic.cn',
    '+.qlogo.cn',
    '+.myqcloud.com',
    '+.qcloud.com',
    '+.tencent.com',
    '+.wechat.com',
    '+.weixin.com',
    '+.tencentcs.com',
    '+.gtimg.com',
    '+.weiyun.com',
    // Alibaba / Alipay
    '+.taobao.com',
    '+.tmall.com',
    '+.alipay.com',
    '+.aliyun.com',
    '+.alibaba.com',
    '+.alicdn.com',
    // Baidu / ByteDance
    '+.baidu.com',
    '+.bdstatic.com',
    '+.bytedance.com',
    '+.douyin.com',
    '+.ixigua.com',
  ];

  /// Generate a complete Clash/mihomo config map ready for YAML serialisation.
  ///
  /// **Rule-selection logic** (per spec):
  /// 1. [customRules] non-empty → subscription mode: use [customProxyGroups]
  ///    (if any), but normalize rule order for Android TUN/fake-ip safety.
  /// 2. [customRules] empty → Loyalsoldier template: generate Auto + 🚀 Proxy
  ///    groups and apply Loyalsoldier rule-providers / rules.
  ///
  /// **DNS**: always uses the app's own DNS config regardless of what the
  /// subscription contained (imported DNS is intentionally ignored).
  ///
  /// [dnsStrategy]: 'split' (default) | 'privacy' | 'legacy'
  /// [dnsMode]: 'fake-ip' (default) | 'redir-host'
  /// [templateId]: 'loyalsoldier_standard' (default) | 'lite' — selects the
  ///   built-in rule-provider template when no [customRules] are provided.
  ///   'lite' uses wujun4code/clash-rules-lite (~68k rules, ~60 MB RAM);
  ///   'loyalsoldier_standard' uses the full Loyalsoldier set (~320k rules).
  static Map<String, dynamic> generate({
    required List<ProxyNode> nodes,
    required String geodataPath,
    bool bypassChina = true,
    String? selectedNodeName,
    List<String> customRules = const [],
    List<Map<String, dynamic>> customProxyGroups = const [],
    Map<String, Map<String, dynamic>> customRuleProviders = const {},
    String dnsStrategy = 'split',
    String dnsMode = 'fake-ip',
    String templateId = 'loyalsoldier_standard',
  }) {
    final out = <String, dynamic>{};

    // ── Base settings ────────────────────────────────────────────────────────
    out['port'] = 7890;
    out['socks-port'] = 7891;
    out['mixed-port'] = 7892;
    out['allow-lan'] = false;
    out['bind-address'] = '127.0.0.1';
    out['mode'] = 'rule';
    out['log-level'] = 'info';
    out['external-controller'] = '127.0.0.1:9090';
    out['unified-delay'] = true;
    out['tcp-concurrent'] = true;
    out['geodata-mode'] = false;
    out['geodata-path'] = geodataPath;

    // ── DNS (always app-managed, never from subscription) ────────────────────
    out['dns'] = _buildDns(geodataPath, dnsStrategy, dnsMode);

    // ── Proxies ──────────────────────────────────────────────────────────────
    final proxies = <Map<String, dynamic>>[];
    final proxyNames = <String>[];
    for (final node in nodes) {
      final p = <String, dynamic>{
        'name': node.name,
        'type': node.type,
        'server': node.server,
        'port': node.port,
      };
      node.raw.forEach((k, v) {
        if (k != 'name' && k != 'type' && k != 'server' && k != 'port') {
          p[k] = v;
        }
      });
      // Preserve the subscription's explicit certificate policy. Omission uses
      // Mihomo's secure default; TLS alone is not permission to skip validation.
      proxies.add(p);
      proxyNames.add(node.name);
    }
    out['proxies'] = proxies;

    // ── Proxy-groups & Rules ─────────────────────────────────────────────────
    final hasCustomRules = customRules.isNotEmpty;

    if (hasCustomRules) {
      // Subscription mode: keep the provider/group intent, but normalize rules
      // that are known to break Android TUN/fake-ip when imported verbatim.
      out['proxy-groups'] = customProxyGroups.isNotEmpty
          ? customProxyGroups
          : _defaultProxyGroups(proxyNames, selectedNodeName);
      final normalizedRules = _normalizeCustomRules(customRules);
      out['rules'] = normalizedRules;
      // Build rule-providers for the custom rules:
      //   1. Start with the subscription's own rule-providers (exact definitions).
      //   2. For any RULE-SET reference not covered by (1), fall back to the
      //      Loyalsoldier CDN definition (handles subscriptions that bundle only
      //      rules without bundling the matching rule-providers block).
      final ruleProviders = <String, dynamic>{...customRuleProviders};
      final loyalsoldierProviders = LoyalsoldierTemplate.ruleProviders();
      for (final rule in normalizedRules) {
        if (rule.startsWith('RULE-SET,')) {
          final name = rule.split(',')[1];
          if (!ruleProviders.containsKey(name) &&
              loyalsoldierProviders.containsKey(name)) {
            ruleProviders[name] = loyalsoldierProviders[name]!;
          }
        }
      }
      if (ruleProviders.isNotEmpty) out['rule-providers'] = ruleProviders;
    } else {
      // Builtin template mode: build standard groups, add rule-providers + rules.
      out['proxy-groups'] = _defaultProxyGroups(proxyNames, selectedNodeName);
      final useLite = templateId == 'lite' || templateId == 'clash_rules_lite';
      out['rule-providers'] = useLite
          ? LiteTemplate.ruleProviders()
          : LoyalsoldierTemplate.ruleProviders();
      out['rules'] = useLite
          ? LiteTemplate.rules(proxyGroup: '🚀 Proxy', bypassChina: bypassChina)
          : LoyalsoldierTemplate.rules(
              proxyGroup: '🚀 Proxy', bypassChina: bypassChina);
    }

    return out;
  }

  // ── helpers ─────────────────────────────────────────────────────────────────

  static List<String> _normalizeCustomRules(List<String> rules) {
    final cleaned = rules
        .map((rule) => rule.trim())
        .where((rule) => rule.isNotEmpty)
        .toList();
    if (cleaned.isEmpty) return cleaned;

    final proxyPolicy = _inferProxyPolicy(cleaned);
    final directPolicy = _inferDirectPolicy(cleaned) ?? 'DIRECT';
    final prefix = <String>[];
    final proxyRules = <String>[];
    final directRules = <String>[];
    final matchRules = <String>[];

    for (final rule in cleaned) {
      if (_isMatchRule(rule)) {
        matchRules.add(rule);
      } else if (_isProxyBoundRule(rule)) {
        proxyRules.add(rule);
      } else if (_isLateDirectRule(rule)) {
        directRules.add(rule);
      } else {
        prefix.add(rule);
      }
    }

    final normalized = <String>[];
    final seen = <String>{};
    void addRule(String rule) {
      if (seen.add(rule)) normalized.add(rule);
    }

    for (final rule in prefix) {
      addRule(rule);
    }
    if (proxyPolicy != null) {
      for (final rule in _googlePlayDownloadRules(proxyPolicy, directPolicy)) {
        addRule(rule);
      }
    }
    for (final rule in proxyRules) {
      addRule(rule);
    }
    for (final rule in directRules) {
      addRule(rule);
    }

    if (proxyPolicy != null) {
      addRule('MATCH,$proxyPolicy');
    } else {
      for (final rule in matchRules) {
        addRule(rule);
      }
    }

    return normalized;
  }

  static String? _inferProxyPolicy(List<String> rules) {
    for (final rule in rules) {
      final parts = rule.split(',');
      if (parts.length >= 3 &&
          parts[0] == 'RULE-SET' &&
          _proxyRuleSetNames.contains(parts[1]) &&
          parts[2] != 'DIRECT' &&
          parts[2] != 'REJECT') {
        return parts[2];
      }
    }
    return null;
  }

  static String? _inferDirectPolicy(List<String> rules) {
    for (final rule in rules) {
      final parts = rule.split(',');
      if (parts.length < 3 || parts[2] == 'REJECT') continue;
      if (parts[0] == 'RULE-SET' &&
          const {'private', 'direct', 'lancidr', 'cncidr'}.contains(parts[1])) {
        return parts[2];
      }
      if (parts[0] == 'GEOSITE' && parts[1].toLowerCase() == 'cn') {
        return parts[2];
      }
      if (parts[0] == 'GEOIP' &&
          const {'LAN', 'CN'}.contains(parts[1].toUpperCase())) {
        return parts[2];
      }
    }
    return null;
  }

  static List<String> _googlePlayDownloadRules(
    String proxyPolicy,
    String directPolicy,
  ) =>
      [
        // Cronet/Google Play can try QUIC first. Android TUN fake-ip may not
        // carry enough domain context for UDP/443 rule matching, so reject
        // QUIC early and let clients retry over TCP, where the domain rules
        // below are reliable.
        'AND,((NETWORK,UDP),(DST-PORT,443)),REJECT',
        'DOMAIN-SUFFIX,googleapis.com,$proxyPolicy',
        'DOMAIN-SUFFIX,googleusercontent.com,$proxyPolicy',
        'DOMAIN-SUFFIX,gvt1.com,$proxyPolicy',
        'DOMAIN-SUFFIX,gvt2.com,$proxyPolicy',
        'DOMAIN-SUFFIX,gvt3.com,$proxyPolicy',
        // Play's download CDN can resolve to nearby Google cache addresses.
        // On China networks those 114.x cache targets are reachable directly,
        // while our overseas HTTP proxy nodes return 503/504 tunneling them.
        'DOMAIN-SUFFIX,xn--ngstr-lra8j.com,$directPolicy',
      ];

  static const _proxyRuleSetNames = {
    'google',
    'proxy',
    'gfw',
    'tld-not-cn',
    'telegramcidr',
  };

  static bool _isProxyBoundRule(String rule) {
    final parts = rule.split(',');
    return parts.length >= 2 &&
        parts[0] == 'RULE-SET' &&
        _proxyRuleSetNames.contains(parts[1]);
  }

  static bool _isLateDirectRule(String rule) {
    final parts = rule.split(',');
    if (parts.isEmpty) return false;
    if (parts[0] == 'RULE-SET' && parts.length >= 2) {
      return parts[1] == 'lancidr' ||
          parts[1] == 'direct' ||
          parts[1] == 'cncidr';
    }
    if (parts[0] == 'GEOSITE' && parts.length >= 2) {
      return parts[1].toLowerCase() == 'cn';
    }
    if (parts[0] == 'GEOIP' && parts.length >= 2) {
      return parts[1].toUpperCase() == 'CN';
    }
    return false;
  }

  static bool _isMatchRule(String rule) => rule.startsWith('MATCH,');

  /// Build the DNS config block based on [strategy] and [mode].
  ///
  /// strategy:
  /// - split   : CN domains → plain ISP DNS (best CDN), international → DoH.
  ///             nameserver-policy requires geosite.dat; auto-falls back to
  ///             legacy if the file is missing.
  /// - privacy : All queries → DoH. ISP sees zero DNS records.
  ///             Also requires geosite.dat; falls back to legacy if missing.
  /// - legacy  : Original fallback-filter behaviour; no nameserver-policy.
  ///
  /// mode: 'fake-ip' | 'redir-host'
  static Map<String, dynamic> _buildDns(
      String geodataPath, String strategy, String mode) {
    final isFakeIp = mode != 'redir-host';
    final base = <String, dynamic>{
      'enable': true,
      'listen': '0.0.0.0:1053',
      // true so a hostname resolution that matches a proxy rule is dialed
      // through that proxy — otherwise it resolves directly over the
      // device's real network path, which is exactly where domestic ISP/
      // GFW-style DNS injection happens. Debug logging on Android caught
      // this precisely: a Google download-CDN redirector hostname resolved
      // to a poisoned domestic IP via the plain-UDP domestic nameservers,
      // which is what made large downloads (Play Store installs) hang
      // indefinitely. See mobile/cfgen/generator.go's matching comment —
      // both native layers set this unconditionally on top of whatever
      // Dart writes, this is kept in sync for consistency/no-native-patch
      // paths (e.g. config preview).
      'respect-rules': true,
      'enhanced-mode': isFakeIp ? 'fake-ip' : 'redir-host',
      // Not 198.18.0.0/15: Loyalsoldier's lancidr.txt (used by
      // LoyalsoldierTemplate/LiteTemplate rule-providers) bundles that exact
      // IANA-reserved block as a "route DIRECT" range, so a fake-ip literal
      // for a domain not covered by a more specific rule-provider falls
      // through to lancidr and gets force-routed DIRECT instead of through
      // the proxy. 28.18.0.1/16 is a real, allocated-but-obscure block (part
      // of 28.0.0.0/8, long assigned to the US DoD network) that lancidr
      // does not bundle — mirrors internal/config/fakeip.go's
      // SelectFakeIPRange on the router side.
      //
      // (Was briefly reverted to 198.18.0.1/16 to diagnose a Google Play
      // download hang; confirmed unrelated — the actual cause was a TUN
      // MTU mismatch, see mobile/cfgen/generator.go's buildAndroidTUN.)
      if (isFakeIp) 'fake-ip-range': '28.18.0.1/16',
      if (isFakeIp) 'fake-ip-filter': _fakeIpFilter,
      'default-nameserver': _cnPlainDns,
      'proxy-server-nameserver': _cnPlainDns,
    };

    // geosite.dat is bundled as an app asset and extracted to filesDir on
    // first VPN start; it is present in normal operation.
    final geositePresent = File('$geodataPath/geosite.dat').existsSync();
    final effective =
        (strategy == 'split' || strategy == 'privacy') && !geositePresent
            ? 'legacy'
            : strategy;
    if (isFakeIp && geositePresent) {
      base['fake-ip-filter'] = ['geosite:cn', ..._fakeIpFilter];
    }

    if (effective == 'privacy') {
      base['nameserver'] = _cnDoh;
      base['nameserver-policy'] = {
        'geosite:cn': _cnDoh,
        'geosite:geolocation-!cn': _intlDoh,
      };
    } else if (effective == 'split') {
      base['nameserver'] = _cnPlainDns;
      base['nameserver-policy'] = {
        'geosite:cn': _cnPlainDns,
        'geosite:geolocation-!cn': _intlDoh,
      };
      base['fallback'] = _intlDoh;
      base['fallback-filter'] = {
        'geoip': true,
        'geoip-code': 'CN',
        'ipcidr': ['240.0.0.0/4'],
      };
    } else {
      // legacy: original fallback-filter behaviour
      base['nameserver'] = ['223.5.5.5', '8.8.8.8'];
      base['fallback'] = [
        'https://1.1.1.1/dns-query',
        'https://8.8.8.8/dns-query',
        'https://doh.pub/dns-query',
        'https://dns.alidns.com/dns-query',
      ];
      base['fallback-filter'] = {
        'geoip': true,
        'geoip-code': 'CN',
        'ipcidr': ['240.0.0.0/4'],
      };
    }

    return base;
  }

  /// Auto url-test group + 🚀 Proxy select group, preserving [selectedNodeName]
  /// at the top of the list.
  static List<Map<String, dynamic>> _defaultProxyGroups(
    List<String> proxyNames,
    String? selectedNodeName,
  ) {
    final selected = selectedNodeName?.trim();
    final ordered = <String>[];
    if (selected != null &&
        selected.isNotEmpty &&
        proxyNames.contains(selected)) {
      ordered.add(selected);
    }
    ordered.addAll(proxyNames.where((n) => n != selected));

    final autoProxies =
        ordered.isNotEmpty ? List<String>.from(ordered) : ['DIRECT'];
    final selectProxies = ordered.isNotEmpty
        ? <String>[...ordered, 'Auto', 'DIRECT']
        : ['DIRECT'];

    return [
      {
        'name': 'Auto',
        'type': 'url-test',
        'url': 'http://www.gstatic.com/generate_204',
        'interval': 300,
        'tolerance': 50,
        'proxies': autoProxies,
      },
      {
        'name': '🚀 Proxy',
        'type': 'select',
        'proxies': selectProxies,
      },
    ];
  }
}
