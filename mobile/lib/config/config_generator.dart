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

    final rules = <String>['GEOIP,private,DIRECT,no-resolve'];
    if (bypassChina) {
      rules.add('GEOSITE,cn,DIRECT');
      rules.add('GEOIP,CN,DIRECT');
    }
    rules.add('MATCH,🚀 Proxy');
    out['rules'] = rules;

    return out;
  }
}
