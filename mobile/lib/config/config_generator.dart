import '../subscription/proxy_node.dart';

class ConfigGenerator {
  static Map<String, dynamic> generate({
    required List<ProxyNode> nodes,
    required String geodataPath,
    bool bypassChina = true,
    String? selectedNodeName,
  }) {
    final out = <String, dynamic>{};

    // Proxy ports only — redir-port/tproxy-port need iptables (root), not available on Android
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

    // DNS — redir-host returns real IPs, avoiding the fake-ip routing loop where
    // proxy server hostnames get fake IPs (198.18.x.x) that mihomo cannot dial.
    out['dns'] = {
      'enable': true,
      'listen': '0.0.0.0:1053',
      // Keep DNS bootstrap/direct lookups out of proxy rules to avoid
      // circular dependency when resolving proxy node hostnames.
      'respect-rules': false,
      'enhanced-mode': 'redir-host',
      'nameserver': ['223.5.5.5', '8.8.8.8'],
      // Bootstrap resolver IPs for DoH/DoT hostname lookup.
      'default-nameserver': ['223.5.5.5', '8.8.8.8'],
      // Proxy node hostname resolver. Startup probe may rewrite this to DoH.
      'proxy-server-nameserver': ['223.5.5.5', '8.8.8.8'],
      // OpenWrt-aligned fallback chain: when upstream UDP returns polluted
      // answers (e.g. fake-IP), mihomo can switch to encrypted resolvers.
      'fallback': [
        'https://1.1.1.1/dns-query',
        'https://8.8.8.8/dns-query',
        'https://doh.pub/dns-query',
        'https://dns.alidns.com/dns-query',
      ],
      'fallback-filter': {
        'geoip': true,
        'geoip-code': 'CN',
        // Treat fake-IP/special ranges as polluted so fallback result wins.
        'ipcidr': ['198.18.0.0/15', '240.0.0.0/4'],
      },
    };

    // Proxies
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
      // Commercial proxy providers typically use private CAs not in the Android
      // trust store. Force skip-cert-verify for TLS-enabled nodes so the tunnel
      // actually works; the user already trusts the provider by importing the sub.
      if (p['tls'] == true) {
        p['skip-cert-verify'] = true;
      }
      proxies.add(p);
      proxyNames.add(node.name);
    }
    out['proxies'] = proxies;

    final selected = selectedNodeName?.trim();
    final orderedProxyNames = <String>[];
    if (selected != null &&
        selected.isNotEmpty &&
        proxyNames.contains(selected)) {
      orderedProxyNames.add(selected);
    }
    orderedProxyNames.addAll(proxyNames.where((name) => name != selected));

    final autoProxies = orderedProxyNames.isNotEmpty
        ? List<String>.from(orderedProxyNames)
        : ['DIRECT'];

    final selectProxies = orderedProxyNames.isNotEmpty
        ? <String>[...orderedProxyNames, 'Auto', 'DIRECT']
        : ['DIRECT'];

    out['proxy-groups'] = [
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

    // NOTE:
    // Do not use GEOIP,private here. In fake-ip/TUN flows, mihomo maps domains to
    // 198.18.0.0/15 (RFC 2544 benchmark range). That range is often classified as
    // "private/special", which would wrongly force most domain traffic to DIRECT.
    // We only bypass true local/LAN ranges so domain rules can still decide proxying.
    final rules = <String>[
      'IP-CIDR,127.0.0.0/8,DIRECT,no-resolve',
      'IP-CIDR,10.0.0.0/8,DIRECT,no-resolve',
      'IP-CIDR,172.16.0.0/12,DIRECT,no-resolve',
      'IP-CIDR,192.168.0.0/16,DIRECT,no-resolve',
      'IP-CIDR,169.254.0.0/16,DIRECT,no-resolve',
      'IP-CIDR,100.64.0.0/10,DIRECT,no-resolve',
      'IP-CIDR6,::1/128,DIRECT,no-resolve',
      'IP-CIDR6,fc00::/7,DIRECT,no-resolve',
      'IP-CIDR6,fe80::/10,DIRECT,no-resolve',
    ];
    if (bypassChina) {
      rules.add('GEOSITE,cn,DIRECT');
      rules.add('GEOIP,CN,DIRECT');
    }
    rules.add('MATCH,🚀 Proxy');
    out['rules'] = rules;

    return out;
  }
}
