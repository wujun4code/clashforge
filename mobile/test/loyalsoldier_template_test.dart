import 'package:flutter_test/flutter_test.dart';
import 'package:clashforge_mobile/config/loyalsoldier_template.dart';

void main() {
  group('lancidr rule order', () {
    // Loyalsoldier's lancidr.txt bundles the IANA-reserved 198.18.0.0/15
    // block alongside genuine private ranges — mihomo's default fake-ip pool
    // sits entirely inside it. If lancidr is matched before domain-based
    // rules, every fake-ip literal gets force-routed DIRECT before mihomo
    // ever consults the domain it was assigned to. lancidr must therefore
    // run after every domain-based proxy rule (google/proxy/gfw/tld-not-cn),
    // so it only catches genuine raw-IP LAN traffic no domain rule claimed.

    test('LoyalsoldierTemplate: lancidr runs after domain-based rules', () {
      final rules = LoyalsoldierTemplate.rules();
      final lancidrIndex =
          rules.indexWhere((r) => r.startsWith('RULE-SET,lancidr,'));
      final domainRules = [
        'RULE-SET,google,',
        'RULE-SET,proxy,',
        'RULE-SET,gfw,',
        'RULE-SET,tld-not-cn,',
      ];

      expect(lancidrIndex, greaterThanOrEqualTo(0),
          reason: 'lancidr rule must be present');
      for (final prefix in domainRules) {
        final domainIndex = rules.indexWhere((r) => r.startsWith(prefix));
        expect(domainIndex, greaterThanOrEqualTo(0),
            reason: '$prefix rule must be present');
        expect(lancidrIndex, greaterThan(domainIndex),
            reason: 'lancidr must run after $prefix, not before it');
      }
    });

    test('LiteTemplate: lancidr runs after domain-based rules', () {
      final rules = LiteTemplate.rules();
      final lancidrIndex =
          rules.indexWhere((r) => r.startsWith('RULE-SET,lancidr,'));
      final domainRules = ['RULE-SET,google,', 'RULE-SET,proxy,'];

      expect(lancidrIndex, greaterThanOrEqualTo(0),
          reason: 'lancidr rule must be present');
      for (final prefix in domainRules) {
        final domainIndex = rules.indexWhere((r) => r.startsWith(prefix));
        expect(domainIndex, greaterThanOrEqualTo(0),
            reason: '$prefix rule must be present');
        expect(lancidrIndex, greaterThan(domainIndex),
            reason: 'lancidr must run after $prefix, not before it');
      }
    });
  });
}
