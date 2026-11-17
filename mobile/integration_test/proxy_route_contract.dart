// Test-only contract shared by the real emulator E2E and offline regressions.
// Never include server addresses, credentials or raw API bodies in failures.
void assertExpectedAnyTlsRoute(
  Map<String, dynamic> data,
  String expectedNode,
) {
  final proxies = data['proxies'];
  if (proxies is! Map<String, dynamic>) {
    throw StateError('Live proxy registry missing');
  }
  final expected = proxies[expectedNode];
  if (expected is! Map<String, dynamic> ||
      (expected['type'] as String? ?? '').toLowerCase() != 'anytls') {
    throw StateError('Expected subscription AnyTLS node missing');
  }
  String? current;
  for (final name in ['🚀 节点选择', '🚀节点选择', '🚀 Proxy', 'Proxy']) {
    if (proxies.containsKey(name)) {
      current = name;
      break;
    }
  }
  final seen = <String>{};
  while (current != null && seen.add(current)) {
    if (current == expectedNode) return;
    final group = proxies[current];
    if (group is! Map<String, dynamic>) break;
    final next = group['now'];
    current = next is String ? next : null;
  }
  throw StateError('Selected route does not reach expected subscription node');
}
