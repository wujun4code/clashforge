/// Loyalsoldier clash-rules template for mobile.
///
/// Rule-providers are fetched by mihomo at runtime (requires internet, which
/// the VPN already needs). Rules reference the [proxyGroup] name so the
/// caller can pass whichever proxy-group name it uses (default: '🚀 Proxy').
///
/// Mirrors the OpenWrt publish/template.go configuration.
class LoyalsoldierTemplate {
  LoyalsoldierTemplate._();

  static const _baseUrl =
      'https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release';

  static Map<String, dynamic> ruleProviders() => {
        'reject': _provider('domain', 'reject'),
        'icloud': _provider('domain', 'icloud'),
        'apple': _provider('domain', 'apple'),
        'google': _provider('domain', 'google'),
        'proxy': _provider('domain', 'proxy'),
        'private': _provider('domain', 'private'),
        'gfw': _provider('domain', 'gfw'),
        'tld-not-cn': _provider('domain', 'tld-not-cn'),
        'telegramcidr': _provider('ipcidr', 'telegramcidr'),
        'cncidr': _provider('ipcidr', 'cncidr'),
        'lancidr': _provider('ipcidr', 'lancidr'),
        'applications': _provider('classical', 'applications'),
      };

  /// Returns the ordered rule list.
  ///
  /// [proxyGroup] — name of the proxy-group that non-direct traffic goes to.
  /// [bypassChina] — whether to route CN domains/IPs directly.
  static List<String> rules({
    String proxyGroup = '🚀 Proxy',
    bool bypassChina = true,
  }) =>
      [
        // Ad blocking
        'RULE-SET,reject,REJECT',

        // Always-direct: private / Apple / iCloud / LAN
        'RULE-SET,private,DIRECT',
        'RULE-SET,applications,DIRECT',
        'RULE-SET,icloud,DIRECT',
        'RULE-SET,apple,DIRECT',
        'RULE-SET,lancidr,DIRECT,no-resolve',

        // Local IP ranges (no-resolve to avoid fake-IP loops in TUN mode)
        'IP-CIDR,127.0.0.0/8,DIRECT,no-resolve',
        'IP-CIDR,10.0.0.0/8,DIRECT,no-resolve',
        'IP-CIDR,172.16.0.0/12,DIRECT,no-resolve',
        'IP-CIDR,192.168.0.0/16,DIRECT,no-resolve',
        'IP-CIDR,169.254.0.0/16,DIRECT,no-resolve',
        'IP-CIDR,100.64.0.0/10,DIRECT,no-resolve',
        'IP-CIDR6,::1/128,DIRECT,no-resolve',
        'IP-CIDR6,fc00::/7,DIRECT,no-resolve',
        'IP-CIDR6,fe80::/10,DIRECT,no-resolve',

        // Proxy-destined traffic
        'RULE-SET,google,$proxyGroup',
        'RULE-SET,proxy,$proxyGroup',
        'RULE-SET,gfw,$proxyGroup',
        'RULE-SET,tld-not-cn,$proxyGroup',
        'RULE-SET,telegramcidr,$proxyGroup,no-resolve',

        // China bypass (conditional)
        if (bypassChina) ...const [
          'GEOSITE,cn,DIRECT',
          'RULE-SET,cncidr,DIRECT,no-resolve',
          'GEOIP,CN,DIRECT',
        ],

        // Catchall
        'MATCH,$proxyGroup',
      ];

  static Map<String, dynamic> _provider(String behavior, String name) => {
        'type': 'http',
        'behavior': behavior,
        'url': '$_baseUrl/$name.txt',
        'path': './rule_provider/$name.yaml',
        'interval': 86400,
      };
}
