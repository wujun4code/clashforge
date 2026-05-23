import 'proxy_node.dart';

/// Result of parsing a subscription string.
///
/// - [proxies]      — always populated; the list of proxy nodes.
/// - [proxyGroups]  — non-empty only when the subscription contained
///                    `proxy-groups:`; these are passed through as-is.
/// - [rules]        — non-empty only when the subscription contained
///                    `rules:`; signals "custom rules" mode.
///
/// When [hasCustomRules] is false the caller should apply the
/// Loyalsoldier template rules instead.
class ParsedSubscription {
  const ParsedSubscription({
    required this.proxies,
    this.proxyGroups = const [],
    this.rules = const [],
  });

  final List<ProxyNode> proxies;
  final List<Map<String, dynamic>> proxyGroups;
  final List<String> rules;

  bool get hasCustomRules => rules.isNotEmpty;
}
