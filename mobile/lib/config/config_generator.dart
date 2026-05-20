import '../subscription/proxy_node.dart';

class ConfigGenerator {
  static Map<String, dynamic> generate({
    required List<ProxyNode> nodes,
    required String geodataPath,
    bool bypassChina = true,
  }) {
    final out = <String, dynamic>{};

    // Proxy ports only — redir-port/tproxy-port need iptables (root), not available on Android
    out['port']       = 7890;
    out['socks-port'] = 7891;
    out['mixed-port'] = 7892;
    out['allow-lan']  = false;
    out['bind-address'] = '127.0.0.1';
    out['mode']       = 'rule';
    out['log-level']  = 'info';
    out['external-controller'] = '127.0.0.1:9090';
    out['unified-delay']  = true;
    out['tcp-concurrent'] = true;
    out['geodata-mode'] = false;
    out['geodata-path'] = geodataPath;

    // DNS — fake-ip mode; mihomo patches the listen address after seeing the TUN fd
    out['dns'] = {
      'enable': true,
      'listen': '0.0.0.0:1053',
      'enhanced-mode': 'fake-ip',
      'nameserver': ['223.5.5.5', '8.8.8.8'],
      'fake-ip-filter': ['*.lan', 'localhost.ptlogin2.qq.com'],
    };

    // Proxies
    final proxies    = <Map<String, dynamic>>[];
    final proxyNames = <String>[];
    for (final node in nodes) {
      final p = <String, dynamic>{
        'name':   node.name,
        'type':   node.type,
        'server': node.server,
        'port':   node.port,
      };
      node.raw.forEach((k, v) {
        if (k != 'name' && k != 'type' && k != 'server' && k != 'port') p[k] = v;
      });
      proxies.add(p);
      proxyNames.add(node.name);
    }
    out['proxies'] = proxies;

    out['proxy-groups'] = [
      {
        'name':     '🚀 Proxy',
        'type':     'url-test',
        'url':      'http://www.gstatic.com/generate_204',
        'interval': 300,
        'proxies':  proxyNames.isNotEmpty ? List<String>.from(proxyNames) : ['DIRECT'],
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
