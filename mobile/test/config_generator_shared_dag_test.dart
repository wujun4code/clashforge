import 'dart:collection';

import 'package:clashforge_mobile/config/config_generator.dart';
import 'package:flutter_test/flutter_test.dart';

// Count real generator payload reads, not elapsed microseconds. Abort runaway
// traversal deterministically so the pre-fix regression cannot hang the suite.
class _ObservedProvider extends MapBase<String, dynamic> {
  _ObservedProvider(this.delegate, this.onRead);

  final Map<String, dynamic> delegate;
  final void Function() onRead;

  @override
  dynamic operator [](Object? key) {
    onRead();
    return delegate[key];
  }

  @override
  void operator []=(String key, dynamic value) => delegate[key] = value;
  @override
  void clear() => delegate.clear();
  @override
  Iterable<String> get keys => delegate.keys;
  @override
  dynamic remove(Object? key) => delegate.remove(key);
}

void main() {
  group('shared provider DAG bounded traversal', () {
    for (final leaf in ['DOMAIN,example.invalid', 'IP-CIDR,192.0.2.0/24']) {
      test('28-node repeated-reference DAG stays off: $leaf', () {
        var reads = 0;
        final providers = <String, Map<String, dynamic>>{};
        for (var i = 27; i >= 0; i--) {
          providers['p$i'] = _ObservedProvider({
            'type': 'inline',
            'behavior': 'classical',
            'payload':
                i == 27 ? [leaf] : ['RULE-SET,p${i + 1}', 'RULE-SET,p${i + 1}'],
          }, () {
            reads++;
            if (reads > 1000) {
              throw StateError('Shared DAG exceeded 1000 provider reads; '
                  'repeated dependencies are being expanded');
            }
          });
        }
        const rules = ['RULE-SET,p0,DIRECT', 'RULE-SET,p1,DIRECT'];
        final config = ConfigGenerator.generate(
          nodes: const [],
          geodataPath: '/nonexistent',
          powerProfile: PowerProfile.battery,
          customRules: rules,
          customRuleProviders: providers,
        );
        expect(config['find-process-mode'], 'off');
        expect(config['rules'], rules);
        expect((config['rule-providers'] as Map)['p0'], same(providers['p0']));
        expect(reads, lessThanOrEqualTo(28 * 3),
            reason: 'each inline provider is inspected once per generation');

        // A subsequent generation must not reuse stale process-free results.
        providers['p27']!['payload'] = ['PROCESS-NAME,org.example.worker'];
        reads = 0;
        final changed = ConfigGenerator.generate(
          nodes: const [],
          geodataPath: '/nonexistent',
          powerProfile: PowerProfile.battery,
          customRules: rules,
          customRuleProviders: providers,
        );
        expect(changed['find-process-mode'], 'strict');
        expect(reads, lessThanOrEqualTo(28 * 3));
      });
    }

    for (final profile in PowerProfile.values) {
      for (final mode in <String?>[null, 'off', 'strict', 'always']) {
        if (profile == PowerProfile.battery && mode == null) continue;
        test('no inference for ${profile.name}, explicit $mode', () {
          var reads = 0;
          final provider = _ObservedProvider({
            'type': 'inline',
            'behavior': 'classical',
            'payload': ['PROCESS-NAME,org.example.worker'],
          }, () => reads++);
          final config = ConfigGenerator.generate(
            nodes: const [],
            geodataPath: '/nonexistent',
            powerProfile: profile,
            findProcessMode: mode,
            customRules: const ['RULE-SET,p0,DIRECT'],
            customRuleProviders: {'p0': provider},
          );
          expect(config['find-process-mode'], mode ?? 'strict');
          expect(reads, 0, reason: 'policy already determines the result');
          expect((config['rule-providers'] as Map)['p0'], same(provider));
        });
      }
    }

    for (final leaf in [
      'PROCESS-NAME,org.example.worker',
      'RULE-SET,missing',
      'RULE-SET,p0',
      'RULE-SET,external',
    ]) {
      test('shared safe branch cannot hide $leaf', () {
        final providers = <String, Map<String, dynamic>>{
          'external': {'type': 'http', 'behavior': 'classical'},
          for (var i = 0; i < 28; i++)
            'p$i': {
              'type': 'inline',
              'behavior': 'classical',
              'payload': i == 27
                  ? ['DOMAIN,example.invalid']
                  : ['RULE-SET,p${i + 1}', 'RULE-SET,p${i + 1}'],
            },
        };
        (providers['p0']!['payload'] as List).add(leaf);
        final config = ConfigGenerator.generate(
          nodes: const [],
          geodataPath: '/nonexistent',
          powerProfile: PowerProfile.battery,
          customRules: const ['RULE-SET,p0,DIRECT'],
          customRuleProviders: providers,
        );
        expect(config['find-process-mode'], 'strict');
      });
    }
  });
}
