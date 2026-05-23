import 'proxy_node.dart';

/// Result of parsing a subscription string.
///
/// - [proxies]        — always populated; the list of proxy nodes.
/// - [proxyGroups]    — non-empty only when the subscription contained
///                      `proxy-groups:`; these are passed through as-is.
/// - [rules]          — non-empty only when the subscription contained
///                      `rules:`; signals "custom rules" mode.
/// - [ruleProviders]  — non-empty only when the subscription contained
///                      `rule-providers:`; passed through in custom rules mode
///                      so mihomo can download the referenced rule sets.
///
/// When [hasCustomRules] is false the caller should apply the
/// Loyalsoldier template rules instead.
class ParsedSubscription {
  const ParsedSubscription({
    required this.proxies,
    this.proxyGroups = const [],
    this.rules = const [],
    this.ruleProviders = const {},
  });

  final List<ProxyNode> proxies;
  final List<Map<String, dynamic>> proxyGroups;
  final List<String> rules;
  final Map<String, Map<String, dynamic>> ruleProviders;

  bool get hasCustomRules => rules.isNotEmpty;
}
