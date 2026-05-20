import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import 'proxy_node.dart';

class SubscriptionStore {
  static const _kNodes    = 'cf_nodes_v1';
  static const _kUrl      = 'cf_sub_url_v1';
  static const _kNickname = 'cf_sub_nickname_v1';

  static Future<void> save(
    List<ProxyNode> nodes, {
    String url = '',
    String nickname = '',
  }) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setStringList(_kNodes, nodes.map((n) => jsonEncode(n.toJson())).toList());
    if (url.isNotEmpty)      await prefs.setString(_kUrl, url);
    if (nickname.isNotEmpty) {
      await prefs.setString(_kNickname, nickname);
      // Increment the per-day counter used by generateDefaultNickname()
      final dateStr = _todayString();
      final key     = 'cf_sub_count_$dateStr';
      await prefs.setInt(key, (prefs.getInt(key) ?? 0) + 1);
    }
  }

  static Future<List<ProxyNode>> loadNodes() async {
    final prefs = await SharedPreferences.getInstance();
    final list = prefs.getStringList(_kNodes) ?? [];
    return list.map((s) => ProxyNode.fromJson(jsonDecode(s) as Map<String, dynamic>)).toList();
  }

  static Future<String> loadUrl() async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getString(_kUrl) ?? '';
  }

  static Future<String> loadNickname() async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getString(_kNickname) ?? '';
  }

  /// Returns a default nickname like "20260520_1", "20260520_2", etc.
  /// Does NOT increment the counter — that happens inside save().
  static Future<String> generateDefaultNickname() async {
    final prefs   = await SharedPreferences.getInstance();
    final dateStr = _todayString();
    final count   = (prefs.getInt('cf_sub_count_$dateStr') ?? 0) + 1;
    return '${dateStr}_$count';
  }

  static String _todayString() {
    final now = DateTime.now();
    return '${now.year}'
        '${now.month.toString().padLeft(2, '0')}'
        '${now.day.toString().padLeft(2, '0')}';
  }
}
