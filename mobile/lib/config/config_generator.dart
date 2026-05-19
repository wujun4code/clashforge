import '../subscription/proxy_node.dart';

class ConfigGenerator {
  static Map<String, dynamic> generate({
    required List<ProxyNode> nodes,
    required String geodataPath,
    bool bypassChina = true,
  }) {
    final Map<String, dynamic> out = {};

    // Basic config
    out['port'] = 7890;
    out['socks-port'] = 7891;
    out['mixed-port'] = 7892;
    out['redir-port'] = 7893;
    out['tproxy-port'] = 7894;
    out['allow-lan'] = false;
    out['bind-address'] = '127.0.0.1';
    out['mode'] = 'rule';
    out['log-level'] = 'info';
    out['external-controller'] = '127.0.0.1:9090';
    out['unified-delay'] = true;
    out['tcp-concurrent'] = true;
    out['geodata-mode'] = false; // mmdb formatcountry.mmdb
    out['geodata-path'] = geodataPath;

    // DNS config as per PRD Section 5
    out['dns'] = {
      'enable': true,
      'enhanced-mode': 'fake-ip',
      'nameserver': [
        '223.5.5.5', // Alidns
        '8.8.8.8',   // Google DNS (over proxy)
      ],
      'fake-ip-filter': [
        '*.lan',
        'localhost.ptlogin2.qq.com',
      ],
    };

    // Proxies list
    final List<Map<String, dynamic>> proxies = [];
    final List<String> proxyNames = [];
    for (final node in nodes) {
      final p = <String, dynamic>{
        'name': node.name,
        'type': node.type,
        'server': node.server,
        'port': node.port,
      };
      node.raw.forEach((key, val) {
        if (key != 'name' && key != 'type' && key != 'server' && key != 'port') {
          p[key] = val;
        }
      });
      proxies.add(p);
      proxyNames.add(node.name);
    }
    out['proxies'] = proxies;

    // Proxy Groups (as per PRD Section 5/9 & generator.go)
    final List<String> autoProxies = proxyNames.isNotEmpty ? List<String>.from(proxyNames) : ['DIRECT'];
    final List<String> selectProxies = ['🚀 Proxy', 'DIRECT', ...proxyNames];

    out['proxy-groups'] = [
      {
        'name': '🚀 Proxy',
        'type': 'url-test',
        'url': 'http://www.gstatic.com/generate_204',
        'interval': 300,
        'proxies': proxyNames.isNotEmpty ? List<String>.from(proxyNames) : ['DIRECT'],
      },
    ];

    // Rules
    final List<String> rules = [
      'GEOIP,private,DIRECT,no-resolve',
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
