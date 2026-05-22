import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import 'proxy_node.dart';

class Subscription {
  final String id;
  final String nickname;
  final String url;
  final List<ProxyNode> nodes;

  const Subscription({
    required this.id,
    required this.nickname,
    required this.url,
    required this.nodes,
  });

  Map<String, dynamic> toJson() => {
        'id': id,
        'nickname': nickname,
        'url': url,
        'nodes': nodes.map((n) => n.toJson()).toList(),
      };

  factory Subscription.fromJson(Map<String, dynamic> json) => Subscription(
        id: json['id'] as String,
        nickname: json['nickname'] as String,
        url: json['url'] as String? ?? '',
        nodes: (json['nodes'] as List)
            .map((n) => ProxyNode.fromJson(n as Map<String, dynamic>))
            .toList(),
      );
}

class SubscriptionStore {
  static const _kNodes    = 'cf_nodes_v1';
  static const _kUrl      = 'cf_sub_url_v1';
  static const _kNickname = 'cf_sub_nickname_v1';
  static const _kSubs     = 'cf_subscriptions_v2';
  static const _kActiveId = 'cf_active_sub_id_v1';

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
