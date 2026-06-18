import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import 'proxy_node.dart';

class Subscription {
  final String id;
  final String nickname;
  final String url;
  final List<ProxyNode> nodes;

  /// Non-empty when the original subscription contained `rules:`.
  /// Empty means "no custom rules" → use Loyalsoldier template at config-gen time.
  final List<String> customRules;

  /// Non-empty when the original subscription contained `proxy-groups:`.
  /// Only used when [hasCustomRules] is true; otherwise the config generator
  /// builds its own proxy-groups from [nodes].
  final List<Map<String, dynamic>> customProxyGroups;

  /// Non-empty when the original subscription contained `rule-providers:`.
  /// Passed through to the config generator so mihomo can download rule sets
  /// referenced by [customRules] (e.g. RULE-SET,chatGPT,...).
  final Map<String, Map<String, dynamic>> customRuleProviders;

  /// True for the app's built-in free-node subscription.
  /// UI must not reveal server/port/protocol for built-in nodes.
  final bool isBuiltIn;

  /// True for subscriptions automatically imported from CLASHFORGE_BUNDLED_SUBS
  /// at build time. [importedAt] is set to the build timestamp, not the runtime
  /// import time.
  final bool isBundled;

  /// When this subscription was imported. For bundled subscriptions this is the
  /// build time (injected via CLASHFORGE_BUILD_TIME at compile time).
  /// For existing subscriptions without a stored timestamp, derived from [id].
  final DateTime importedAt;

  Subscription({
    required this.id,
    required this.nickname,
    required this.url,
    required this.nodes,
    this.customRules = const [],
    this.customProxyGroups = const [],
    this.customRuleProviders = const {},
    this.isBuiltIn = false,
    this.isBundled = false,
    DateTime? importedAt,
  }) : importedAt = importedAt ?? DateTime.now();

  bool get hasCustomRules => customRules.isNotEmpty;

  Map<String, dynamic> toJson() => {
        'id': id,
        'nickname': nickname,
        'url': url,
        'nodes': nodes.map((n) => n.toJson()).toList(),
        if (customRules.isNotEmpty) 'custom_rules': customRules,
        if (customProxyGroups.isNotEmpty)
          'custom_proxy_groups': customProxyGroups,
        if (customRuleProviders.isNotEmpty)
          'custom_rule_providers': customRuleProviders,
        if (isBuiltIn) 'is_built_in': true,
        if (isBundled) 'is_bundled': true,
        'imported_at': importedAt.millisecondsSinceEpoch,
      };

  factory Subscription.fromJson(Map<String, dynamic> json) {
    final id = json['id'] as String;
    return Subscription(
      id: id,
      nickname: json['nickname'] as String,
      url: json['url'] as String? ?? '',
      nodes: (json['nodes'] as List)
          .map((n) => ProxyNode.fromJson(n as Map<String, dynamic>))
          .toList(),
      customRules: (json['custom_rules'] as List?)
              ?.map((e) => e as String)
              .toList() ??
          const [],
      customProxyGroups: (json['custom_proxy_groups'] as List?)
              ?.map((e) => Map<String, dynamic>.from(e as Map))
              .toList() ??
          const [],
      customRuleProviders: (json['custom_rule_providers'] as Map?)?.map(
            (k, v) =>
                MapEntry(k as String, Map<String, dynamic>.from(v as Map)),
          ) ??
          const {},
      isBuiltIn: json['is_built_in'] as bool? ?? false,
      isBundled: json['is_bundled'] as bool? ?? false,
      // Backward compat: if imported_at absent, derive from id (epoch ms string)
      importedAt: json['imported_at'] != null
          ? DateTime.fromMillisecondsSinceEpoch(json['imported_at'] as int)
          : _importedAtFromId(id),
    );
  }

  static DateTime _importedAtFromId(String id) {
    try {
      return DateTime.fromMillisecondsSinceEpoch(int.parse(id));
    } catch (_) {
      return DateTime.now();
    }
  }
}

class SubscriptionStore {
  static const _kNodes    = 'cf_nodes_v1';
  static const _kUrl      = 'cf_sub_url_v1';
  static const _kNickname = 'cf_sub_nickname_v1';
  static const _kSubs     = 'cf_subscriptions_v2';
  static const _kActiveId = 'cf_active_sub_id_v1';

  /// URLs that were ever auto-imported (builtin or bundled).
  /// Once a URL is in this set, auto-import won't re-import it even if the user
  /// deletes the subscription — respecting the user's decision to remove it.
  static const _kEverImportedUrls = 'cf_auto_imported_urls_v1';

  // ── Multi-subscription APIs ──────────────────────────────────

  static Future<void> saveSubscriptions(List<Subscription> subs) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setStringList(
        _kSubs, subs.map((s) => jsonEncode(s.toJson())).toList());
  }

  /// Loads all saved subscriptions. Migrates from legacy v1 format on first run.
  static Future<List<Subscription>> loadSubscriptions() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getStringList(_kSubs);
    if (raw != null && raw.isNotEmpty) {
      return raw
          .map((s) => Subscription.fromJson(
              jsonDecode(s) as Map<String, dynamic>))
          .toList();
    }
    // Migrate from legacy single-sub storage (cf_nodes_v1)
    final nodeStrings = prefs.getStringList(_kNodes) ?? [];
    if (nodeStrings.isNotEmpty) {
      final nodes = nodeStrings
          .map((s) =>
              ProxyNode.fromJson(jsonDecode(s) as Map<String, dynamic>))
          .toList();
      final url      = prefs.getString(_kUrl) ?? '';
      final nickname = prefs.getString(_kNickname) ?? '';
      final sub = Subscription(
        id: '${DateTime.now().millisecondsSinceEpoch}',
        nickname: nickname.isEmpty ? _todayString() : nickname,
        url: url,
        nodes: nodes,
      );
      await saveSubscriptions([sub]);
      return [sub];
    }
    return [];
  }

  static Future<void> saveActiveId(String id) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_kActiveId, id);
  }

  static Future<String?> loadActiveId() async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getString(_kActiveId);
  }

  // ── Ever-imported URL tracking ────────────────────────────────
  // Used to prevent auto-import from re-adding subs the user explicitly deleted.

  static Future<Set<String>> loadEverImportedUrls() async {
    final prefs = await SharedPreferences.getInstance();
    return (prefs.getStringList(_kEverImportedUrls) ?? []).toSet();
  }

  static Future<void> markUrlAutoImported(String url) async {
    final prefs = await SharedPreferences.getInstance();
    final current = (prefs.getStringList(_kEverImportedUrls) ?? []).toSet();
    if (current.add(url)) {
      await prefs.setStringList(_kEverImportedUrls, current.toList());
    }
  }

  // ── Legacy single-sub APIs (kept for migration reads) ────────

  static Future<List<ProxyNode>> loadNodes() async {
    final prefs = await SharedPreferences.getInstance();
    final list = prefs.getStringList(_kNodes) ?? [];
    return list
        .map((s) =>
            ProxyNode.fromJson(jsonDecode(s) as Map<String, dynamic>))
        .toList();
  }

  static Future<String> loadUrl() async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getString(_kUrl) ?? '';
  }

  /// Returns a default nickname like "20260520_1", "20260520_2", etc.
  static Future<String> generateDefaultNickname() async {
    final prefs   = await SharedPreferences.getInstance();
    final dateStr = _todayString();
    final count   = (prefs.getInt('cf_sub_count_$dateStr') ?? 0) + 1;
    await prefs.setInt('cf_sub_count_$dateStr', count);
    return '${dateStr}_$count';
  }

  static String _todayString() {
    final now = DateTime.now();
    return '${now.year}'
        '${now.month.toString().padLeft(2, '0')}'
        '${now.day.toString().padLeft(2, '0')}';
  }
}
