import 'dart:convert';
import 'package:http/http.dart' as http;

class UpdateInfo {
  final String tag;
  final String name;
  final String htmlUrl;
  final String body;

  const UpdateInfo({
    required this.tag,
    required this.name,
    required this.htmlUrl,
    required this.body,
  });

  factory UpdateInfo.fromJson(Map<String, dynamic> j) => UpdateInfo(
        tag: j['tag_name'] as String,
        name: j['name'] as String? ?? '',
        htmlUrl: j['html_url'] as String,
        body: j['body'] as String? ?? '',
      );

  // Returns true if this release tag differs from the running app version.
  // currentVersion is the versionName from PackageInfo (e.g. "0.1.0-rc.52").
  bool isNewerThan(String currentVersion) {
    final tagVer = tag.startsWith('v') ? tag.substring(1) : tag;
    return tagVer != currentVersion;
  }
}

Future<UpdateInfo?> fetchLatestRelease() async {
  try {
    final resp = await http.get(
      Uri.parse(
          'https://api.github.com/repos/wujun4code/clashforge/releases/latest'),
      headers: {'Accept': 'application/vnd.github+json'},
    ).timeout(const Duration(seconds: 15));
    if (resp.statusCode != 200) return null;
    final data = jsonDecode(resp.body) as Map<String, dynamic>;
    return UpdateInfo.fromJson(data);
  } catch (_) {
    return null;
  }
}
