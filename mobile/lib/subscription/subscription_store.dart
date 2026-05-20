import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import 'proxy_node.dart';

class SubscriptionStore {
  static const _kNodes = 'cf_nodes_v1';
  static const _kUrl   = 'cf_sub_url_v1';

  static Future<void> save(List<ProxyNode> nodes, {String url = ''}) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setStringList(_kNodes, nodes.map((n) => jsonEncode(n.toJson())).toList());
    if (url.isNotEmpty) await prefs.setString(_kUrl, url);
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
}
