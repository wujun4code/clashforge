import 'package:flutter_test/flutter_test.dart';
import 'package:clashforge_mobile/config/config_generator.dart';
import 'package:clashforge_mobile/subscription/proxy_node.dart';

void main() {
  group('ConfigGenerator Tests', () {
    test('Generate standard config with proxies and rules', () {
      final nodes = [
        ProxyNode(
          name: 'US-01',
          type: 'ss',
          server: '1.2.3.4',
          port: 443,
          raw: {'name': 'US-01', 'type': 'ss', 'server': '1.2.3.4', 'port': 443, 'cipher': 'aes-256-gcm', 'password': 'pwd'},
        )
      ];

      final config = ConfigGenerator.generate(
        nodes: nodes,
        geodataPath: '/data/user/0/com.clashforge.mobile/app_flutter/geodata',
        bypassChina: true,
      );

      expect(config['mode'], equals('rule'));
      expect(config['proxies'], isNotEmpty);
      expect(config['proxies'][0]['name'], equals('US-01'));
      expect(config['proxies'][0]['cipher'], equals('aes-256-gcm'));
      
      final groups = config['proxy-groups'] as List;
      expect(groups[0]['name'], equals('🚀 Proxy'));
      expect(groups[0]['type'], equals('url-test'));

      final rules = config['rules'] as List;
      expect(rules, contains('GEOIP,private,DIRECT,no-resolve'));
      expect(rules, contains('GEOSITE,cn,DIRECT'));
      expect(rules, contains('GEOIP,CN,DIRECT'));
      expect(rules, contains('MATCH,🚀 Proxy'));
    });
  });
}
