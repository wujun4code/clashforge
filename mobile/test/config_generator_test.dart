import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:clashforge_mobile/config/config_generator.dart';
import 'package:clashforge_mobile/subscription/proxy_node.dart';

List<ProxyNode> _twoNodes() => [
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
          'password': 'pwd',
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
          'password': 'pwd2',
        },
      ),
    ];

void main() {
  group('ConfigGenerator Tests', () {
    test('Generate standard config with proxies and rules', () {
      final config = ConfigGenerator.generate(
        nodes: _twoNodes(),
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

      // geosite.dat does not exist at the test path → effective strategy is
      // legacy; default-nameserver / proxy-server-nameserver are _cnPlainDns.
      final dns = config['dns'] as Map<String, dynamic>;
      expect(dns['respect-rules'], isFalse);
      expect(dns['enhanced-mode'], equals('fake-ip'));
      expect(dns['fake-ip-range'], equals('198.18.0.0/15'));
      expect(dns['default-nameserver'], equals(['223.5.5.5', '119.29.29.29']));
      expect(dns['proxy-server-nameserver'], equals(['223.5.5.5', '119.29.29.29']));
      expect((dns['fallback'] as List), contains('https://1.1.1.1/dns-query'));
      expect((dns['fallback'] as List), contains('https://8.8.8.8/dns-query'));
      final fallbackFilter = dns['fallback-filter'] as Map<String, dynamic>;
      expect(fallbackFilter['geoip'], isTrue);
      expect(fallbackFilter['geoip-code'], equals('CN'));
      expect((fallbackFilter['ipcidr'] as List), contains('240.0.0.0/4'));

      final rules = config['rules'] as List;
      expect(rules, contains('IP-CIDR,10.0.0.0/8,DIRECT,no-resolve'));
      expect(rules, contains('IP-CIDR,172.16.0.0/12,DIRECT,no-resolve'));
      expect(rules, contains('IP-CIDR,192.168.0.0/16,DIRECT,no-resolve'));
      expect(rules, contains('GEOSITE,cn,DIRECT'));
      expect(rules, contains('GEOIP,CN,DIRECT'));
      expect(rules, contains('MATCH,🚀 Proxy'));
    });

    test('DNS strategy: legacy — no nameserver-policy, fallback-filter present', () {
      final config = ConfigGenerator.generate(
        nodes: _twoNodes(),
        geodataPath: '/nonexistent',
        dnsStrategy: 'legacy',
      );
      final dns = config['dns'] as Map<String, dynamic>;
      expect(dns['enhanced-mode'], equals('fake-ip'));
      expect(dns['nameserver'], equals(['223.5.5.5', '8.8.8.8']));
      expect(dns['default-nameserver'], equals(['223.5.5.5', '119.29.29.29']));
      expect(dns.containsKey('nameserver-policy'), isFalse);
      expect((dns['fallback'] as List), contains('https://1.1.1.1/dns-query'));
      final ff = dns['fallback-filter'] as Map<String, dynamic>;
      expect(ff['geoip-code'], equals('CN'));
    });

    test('DNS strategy: split/privacy without geosite.dat auto-downgrades to legacy', () {
      for (final strategy in ['split', 'privacy']) {
        final config = ConfigGenerator.generate(
          nodes: _twoNodes(),
          geodataPath: '/nonexistent',
          dnsStrategy: strategy,
        );
        final dns = config['dns'] as Map<String, dynamic>;
        expect(dns.containsKey('nameserver-policy'), isFalse,
            reason: '$strategy without geosite.dat should not emit nameserver-policy');
        expect(dns['nameserver'], equals(['223.5.5.5', '8.8.8.8']),
            reason: '$strategy should fall back to legacy nameserver');
      }
    });

    test('DNS strategy: split with geosite.dat — nameserver-policy uses plain ISP DNS', () {
      final tmp = Directory.systemTemp.createTempSync('geosite_test_');
      final geosite = File('${tmp.path}/geosite.dat')..writeAsBytesSync([0]);
      try {
        final config = ConfigGenerator.generate(
          nodes: _twoNodes(),
          geodataPath: tmp.path,
          dnsStrategy: 'split',
        );
        final dns = config['dns'] as Map<String, dynamic>;
        expect(dns['nameserver'], equals(['223.5.5.5', '119.29.29.29']));
        expect(dns.containsKey('nameserver-policy'), isTrue);
        final policy = dns['nameserver-policy'] as Map<String, dynamic>;
        expect(policy['geosite:cn'], equals(['223.5.5.5', '119.29.29.29']));
        expect(policy['geosite:geolocation-!cn'],
            equals(['https://1.1.1.1/dns-query', 'https://8.8.8.8/dns-query']));
        expect((dns['fallback'] as List), contains('https://1.1.1.1/dns-query'));
      } finally {
        geosite.deleteSync();
        tmp.deleteSync();
      }
    });

    test('DNS strategy: privacy with geosite.dat — all queries via DoH', () {
      final tmp = Directory.systemTemp.createTempSync('geosite_test_');
      final geosite = File('${tmp.path}/geosite.dat')..writeAsBytesSync([0]);
      try {
        final config = ConfigGenerator.generate(
          nodes: _twoNodes(),
          geodataPath: tmp.path,
          dnsStrategy: 'privacy',
        );
        final dns = config['dns'] as Map<String, dynamic>;
        expect(dns['nameserver'],
            equals(['https://dns.alidns.com/dns-query', 'https://doh.pub/dns-query']));
        expect(dns.containsKey('nameserver-policy'), isTrue);
        final policy = dns['nameserver-policy'] as Map<String, dynamic>;
        expect(policy['geosite:cn'],
            equals(['https://dns.alidns.com/dns-query', 'https://doh.pub/dns-query']));
        expect(policy['geosite:geolocation-!cn'],
            equals(['https://1.1.1.1/dns-query', 'https://8.8.8.8/dns-query']));
        expect(dns.containsKey('fallback'), isFalse);
      } finally {
        geosite.deleteSync();
        tmp.deleteSync();
      }
    });
  });
}
