import 'loyalsoldier_template.dart';
import '../subscription/proxy_node.dart';

class ConfigGenerator {
  /// Generate a complete Clash/mihomo config map ready for YAML serialisation.
  ///
  /// **Rule-selection logic** (per spec):
  /// 1. [customRules] non-empty → pass-through mode: use [customProxyGroups]
  ///    (if any) and [customRules] exactly as imported.
  /// 2. [customRules] empty → Loyalsoldier template: generate Auto + 🚀 Proxy
  ///    groups and apply Loyalsoldier rule-providers / rules.
  ///
  /// **DNS**: always uses the app's own DNS config regardless of what the
  /// subscription contained (imported DNS is intentionally ignored).
  static Map<String, dynamic> generate({
    required List<ProxyNode> nodes,
    required String geodataPath,
    bool bypassChina = true,
    String? selectedNodeName,
    List<String> customRules = const [],
    List<Map<String, dynamic>> customProxyGroups = const [],
    Map<String, Map<String, dynamic>> customRuleProviders = const {},
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
    // fake-ip mode: mihomo returns a synthetic 198.18.x.x IP immediately for
    // any query, stores the domain→fakeIP mapping, and resolves the real IP
    // only when establishing the upstream connection. For proxied domains
    // (github.com, youtube.com, …) the proxy does remote DNS — completely
    // bypassing GFW-polluted local resolvers. This eliminates the
    // DNS_PROBE_FINISHED_BAD_CONFIG failure Chrome gets in redir-host mode
    // when 223.5.5.5 or 8.8.8.8 return blocked/poisoned results.
    out['dns'] = {
      'enable': true,
      'listen': '0.0.0.0:1053',
      'respect-rules': false,
      'enhanced-mode': 'fake-ip',
      'fake-ip-range': '198.18.0.0/15',
      'fake-ip-filter': [
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
      ],
      'nameserver': ['223.5.5.5', '8.8.8.8'],
      'default-nameserver': ['223.5.5.5', '8.8.8.8'],
      'proxy-server-nameserver': ['223.5.5.5', '8.8.8.8'],
      'fallback': [
        'https://1.1.1.1/dns-query',
        'https://8.8.8.8/dns-query',
        'https://doh.pub/dns-query',
        'https://dns.alidns.com/dns-query',
      ],
      'fallback-filter': {
        'geoip': true,
        'geoip-code': 'CN',
        'ipcidr': ['240.0.0.0/4'],
      },
    };

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
        if (k != 'name' && k != 'type' && k != 'server' && k != 'port') p[k] = v;
      });
      if (p['tls'] == true) p['skip-cert-verify'] = true;
      proxies.add(p);
      proxyNames.add(node.name);
    }
    out['proxies'] = proxies;

    // ── Proxy-groups & Rules ─────────────────────────────────────────────────
    final hasCustomRules = customRules.isNotEmpty;

    if (hasCustomRules) {
      // Pass-through mode: honour subscription's own groups and rules.
      out['proxy-groups'] = customProxyGroups.isNotEmpty
          ? customProxyGroups
          : _defaultProxyGroups(proxyNames, selectedNodeName);
      out['rules'] = customRules;
      // Build rule-providers for the custom rules:
      //   1. Start with the subscription's own rule-providers (exact definitions).
      //   2. For any RULE-SET reference not covered by (1), fall back to the
      //      Loyalsoldier CDN definition (handles subscriptions that bundle only
      //      rules without bundling the matching rule-providers block).
      final ruleProviders = <String, dynamic>{...customRuleProviders};
      final loyalsoldierProviders = LoyalsoldierTemplate.ruleProviders();
      for (final rule in customRules) {
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
      // Loyalsoldier mode: build standard groups, add rule-providers + template.
      out['proxy-groups'] = _defaultProxyGroups(proxyNames, selectedNodeName);
      out['rule-providers'] = LoyalsoldierTemplate.ruleProviders();
      out['rules'] = LoyalsoldierTemplate.rules(
        proxyGroup: '🚀 Proxy',
        bypassChina: bypassChina,
      );
    }

    return out;
  }

  // ── helpers ─────────────────────────────────────────────────────────────────

  /// Auto url-test group + 🚀 Proxy select group, preserving [selectedNodeName]
  /// at the top of the list.
  static List<Map<String, dynamic>> _defaultProxyGroups(
    List<String> proxyNames,
    String? selectedNodeName,
  ) {
    final selected = selectedNodeName?.trim();
    final ordered = <String>[];
    if (selected != null && selected.isNotEmpty && proxyNames.contains(selected)) {
      ordered.add(selected);
    }
    ordered.addAll(proxyNames.where((n) => n != selected));

    final autoProxies = ordered.isNotEmpty ? List<String>.from(ordered) : ['DIRECT'];
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
