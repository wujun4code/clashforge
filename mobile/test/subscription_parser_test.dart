import 'package:flutter_test/flutter_test.dart';
import 'package:clashforge_mobile/subscription/subscription_parser.dart';

void main() {
  group('SubscriptionParser Tests', () {
    test('Parse raw Clash YAML with proxies block', () {
      const yaml = '''
proxies:
  - name: "HK-01"
    type: ss
    server: "hk01.node.com"
    port: 443
    cipher: "aes-256-gcm"
    password: "pass"
  - name: "US-01"
    type: trojan
    server: "us01.node.com"
    port: 443
    password: "pass"
''';
      final nodes = SubscriptionParser.parse(yaml).proxies;
      expect(nodes.length, 2);
      expect(nodes[0].name, 'HK-01');
      expect(nodes[0].type, 'ss');
      expect(nodes[0].server, 'hk01.node.com');
      expect(nodes[1].name, 'US-01');
      expect(nodes[1].type, 'trojan');
    });

    test('Parse bare YAML sequence list', () {
      const yaml = '''
- name: "SG-01"
  type: trojan
  server: "sg01.node.com"
  port: 888
- name: "JP-01"
  type: ss
  server: "jp01.node.com"
  port: 1080
''';
      final nodes = SubscriptionParser.parse(yaml).proxies;
      expect(nodes.length, 2);
      expect(nodes[0].name, 'SG-01');
      expect(nodes[1].name, 'JP-01');
    });

    test('Parse pasted node block wrapped in markdown fence', () {
      const yaml = '''```yaml
    - name: sg-sg01
      password: xyp8KDrrnqFFk4JZ34U
      port: 443
      server: market.weichichibaole.com
      skip-cert-verify: false
      tls: true
      type: http
      username: u_98f2bcd7
```''';
      final nodes = SubscriptionParser.parse(yaml).proxies;
      expect(nodes.length, 1);
      expect(nodes[0].name, 'sg-sg01');
      expect(nodes[0].type, 'http');
      expect(nodes[0].server, 'market.weichichibaole.com');
      expect(nodes[0].port, 443);
    });

    test('Parse single YAML map node (without list dash)', () {
      const yaml = '''
name: hk-hk01
type: ss
server: hk01.node.com
port: 443
cipher: aes-256-gcm
password: pass
''';
      final nodes = SubscriptionParser.parse(yaml).proxies;
      expect(nodes.length, 1);
      expect(nodes[0].name, 'hk-hk01');
      expect(nodes[0].type, 'ss');
    });

    test('Parse Trojan URI', () {
      const trojanUri = 'trojan://password123@us.trojan.com:443#US-Trojan-Node';
      final nodes = SubscriptionParser.parse(trojanUri).proxies;
      expect(nodes.length, 1);
      expect(nodes[0].name, 'US-Trojan-Node');
      expect(nodes[0].type, 'trojan');
      expect(nodes[0].server, 'us.trojan.com');
      expect(nodes[0].port, 443);
    });
  });
}
