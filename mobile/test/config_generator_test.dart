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
          raw: {
            'name': 'US-01',
            'type': 'ss',
            'server': '1.2.3.4',
            'port': 443,
            'cipher': 'aes-256-gcm',
            'password': 'pwd'
          },
        ),
        ProxyNode(
          name: 'JP-01',
          type: 'ss',
          server: '5.6.7.8',
          port: 443,
          raw: {
            'name': 'JP-01',
            'type': 'ss',
            'server': '5.6.7.8',
            'port': 443,
            'cipher': 'aes-256-gcm',
            'password': 'pwd2'
          },
        ),
      ];

      final config = ConfigGenerator.generate(
        nodes: nodes,
        geodataPath: '/data/user/0/com.clashforge.mobile/app_flutter/geodata',
        bypassChina: true,
        selectedNodeName: 'JP-01',
      );

      expect(config['mode'], equals('rule'));
      expect(config['proxies'], isNotEmpty);
      expect(config['proxies'][0]['name'], equals('US-01'));
      expect(config['proxies'][0]['cipher'], equals('aes-256-gcm'));

      final groups = config['proxy-groups'] as List;
      expect(groups[0]['name'], equals('Auto'));
      expect(groups[0]['type'], equals('url-test'));
      expect(groups[1]['name'], equals('🚀 Proxy'));
      expect(groups[1]['type'], equals('select'));
      expect(
        groups[1]['proxies'],
        equals(['JP-01', 'US-01', 'Auto', 'DIRECT']),
      );

      final dns = config['dns'] as Map<String, dynamic>;
      expect(dns['respect-rules'], isFalse);
      expect(dns['enhanced-mode'], equals('redir-host'));
      expect(dns['default-nameserver'], equals(['223.5.5.5', '8.8.8.8']));
      expect(dns['proxy-server-nameserver'], equals(['223.5.5.5', '8.8.8.8']));

      final rules = config['rules'] as List;
      expect(rules, contains('GEOIP,private,DIRECT,no-resolve'));
      expect(rules, contains('GEOSITE,cn,DIRECT'));
      expect(rules, contains('GEOIP,CN,DIRECT'));
      expect(rules, contains('MATCH,🚀 Proxy'));
    });
  });
}
