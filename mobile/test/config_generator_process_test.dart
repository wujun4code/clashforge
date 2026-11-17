import 'package:flutter_test/flutter_test.dart';
import 'package:clashforge_mobile/config/config_generator.dart';

Map<String, dynamic> batteryConfig(
  List<String> rules, {
  Map<String, Map<String, dynamic>> providers = const {},
  String? templateId,
}) =>
    ConfigGenerator.generate(
      nodes: const [],
      geodataPath: '/nonexistent',
      powerProfile: PowerProfile.battery,
      customRules: rules,
      customRuleProviders: providers,
      templateId: templateId,
    );

void main() {
  group('battery process dependency regression', () {
    for (final rule in [
      'PROCESS-NAME,org.example.worker,DIRECT',
      'PROCESS-PATH,/usr/bin/worker,DIRECT',
      'AND,((PROCESS-NAME,org.example.worker),(DOMAIN,example.invalid)),DIRECT',
      'OR,((DOMAIN,example.invalid),(PROCESS-NAME,org.example.worker)),DIRECT',
      'NOT,((PROCESS-NAME,org.example.worker)),DIRECT',
      'AND,((DOMAIN,example.invalid),(OR,((NETWORK,TCP),(NOT,((PROCESS-NAME,org.example.worker)))))),DIRECT',
    ]) {
      test('preserves process matching: $rule', () {
        final rules = [rule, 'MATCH,DIRECT'];
        final config = batteryConfig(rules);
        expect(config['find-process-mode'], 'strict');
        expect(config['rules'], rules);
      });
    }

    test('inline classical provider retains process payload and rule', () {
      const rules = ['RULE-SET,work-apps,DIRECT', 'MATCH,DIRECT'];
      const providers = {
        'work-apps': {
          'type': 'inline',
          'behavior': 'classical',
          'payload': ['PROCESS-NAME,org.example.worker'],
        },
      };
      final config = batteryConfig(rules, providers: providers);
      expect(config['find-process-mode'], 'strict');
      expect(config['rules'], rules);
      expect(config['rule-providers'], providers);
    });

    test('nested provider reference retains process dependency', () {
      const rule =
          'AND,((DOMAIN,example.invalid),(RULE-SET,work-apps)),DIRECT';
      const providers = {
        'work-apps': {
          'type': 'inline',
          'behavior': 'classical',
          'payload': ['NOT,((PROCESS-NAME,org.example.worker))'],
        },
      };
      final config = batteryConfig([rule], providers: providers);
      expect(config['find-process-mode'], 'strict');
      expect(config['rules'], [rule]);
      expect(config['rule-providers'], providers);
    });

    for (final type in ['http', 'file']) {
      test('unknown external classical $type content stays strict', () {
        final providers = {
          'external': {
            'type': type,
            'behavior': 'classical',
            'url': 'https://example.invalid/rules.yaml',
            'path': './external.yaml',
          },
        };
        final config = batteryConfig(['RULE-SET,external,DIRECT'],
            providers: providers);
        expect(config['find-process-mode'], 'strict');
        expect(config['rule-providers'], providers);
      });
    }

    test('missing provider is unknown, not evidence of no process rules', () {
      final config = batteryConfig(['RULE-SET,unknown,DIRECT']);
      expect(config['find-process-mode'], 'strict');
      expect(config['rules'], ['RULE-SET,unknown,DIRECT']);
    });

    test('provider cycle terminates conservatively', () {
      const providers = {
        'cycle': {
          'type': 'inline',
          'behavior': 'classical',
          'payload': ['RULE-SET,cycle'],
        },
      };
      final config =
          batteryConfig(['RULE-SET,cycle,DIRECT'], providers: providers);
      expect(config['find-process-mode'], 'strict');
      expect(config['rule-providers'], providers);
    });

    test('explicit standard template keeps applications process dependency', () {
      final config = batteryConfig([], templateId: 'loyalsoldier_standard');
      expect(config['find-process-mode'], 'strict');
      expect(config['rules'], contains('RULE-SET,applications,DIRECT'));
      expect(config['rule-providers'], contains('applications'));
    });

    test('domain-only direct and logical rules can disable process lookup', () {
      const rules = [
        'DOMAIN-SUFFIX,example.invalid,DIRECT',
        'AND,((DOMAIN,example.invalid),(NOT,((NETWORK,UDP)))),DIRECT',
        'MATCH,DIRECT',
      ];
      final config = batteryConfig(rules);
      expect(config['find-process-mode'], 'off');
      expect(config['rules'], rules);
    });

    test('known domain/ipcidr and process-free inline providers stay off', () {
      const providers = {
        'domains': {'type': 'http', 'behavior': 'domain'},
        'ips': {'type': 'file', 'behavior': 'ipcidr'},
        'classical': {
          'type': 'inline',
          'behavior': 'classical',
          'payload': ['DOMAIN-SUFFIX,example.invalid'],
        },
        'unused-processes': {'type': 'http', 'behavior': 'classical'},
      };
      const rules = [
        'RULE-SET,domains,DIRECT',
        'RULE-SET,ips,DIRECT',
        'RULE-SET,classical,DIRECT',
      ];
      final config = batteryConfig(rules, providers: providers);
      expect(config['find-process-mode'], 'off');
      expect(config['rules'], rules);
      expect(config['rule-providers'], providers);
    });
  });
}
