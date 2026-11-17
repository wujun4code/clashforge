import 'package:flutter_test/flutter_test.dart';
import 'package:clashforge_mobile/config/config_generator.dart';
import 'package:clashforge_mobile/subscription/proxy_node.dart';

void main() {
  for (final type in [
    'vmess',
    'vless',
    'trojan',
    'http',
    'hysteria2',
    'tuic'
  ]) {
    for (final tls in [true, false, null]) {
      for (final skipVerify in [false, true, null]) {
        test('$type tls=$tls preserves certificate policy $skipVerify', () {
          final raw = <String, dynamic>{
            'name': 'fixture',
            'type': type,
            'server': 'proxy.example.test',
            'port': 443,
            if (tls != null) 'tls': tls,
            if (skipVerify != null) 'skip-cert-verify': skipVerify,
            'sni': 'certificate.example.test',
            'client-fingerprint': 'chrome',
          };
          final original = Map<String, dynamic>.from(raw);
          final config = ConfigGenerator.generate(
            nodes: [ProxyNode.fromJson(raw)],
            geodataPath: '/nonexistent/tls-fixture',
          );
          final proxy = (config['proxies'] as List).single as Map;
          // An absent setting uses Mihomo's secure default, not a new opt-out.
          expect(proxy, original);
          expect(raw, original,
              reason: 'input subscription must not be mutated');
        });
      }
    }
  }
}
