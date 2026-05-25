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

  // Returns true if this release tag is newer than the running app version.
  // currentVersion is the versionName from PackageInfo (e.g. "0.1.0-rc.52").
  bool isNewerThan(String currentVersion) {
    final tagVer = tag.startsWith('v') ? tag.substring(1) : tag;
    final latest = _SemVersion.tryParse(tagVer);
    final current = _SemVersion.tryParse(currentVersion);
    if (latest == null || current == null) {
      // Keep backward compatibility for unexpected non-semver strings.
      return tagVer != currentVersion;
    }

    // Historical migration: rc -> beta should surface as an upgrade
    // even though strict semver orders rc above beta under the same core triplet.
    if (latest.isRcToBetaMigrationFrom(current)) return true;

    return latest.compareTo(current) > 0;
  }
}

class _SemVersion implements Comparable<_SemVersion> {
  final int major;
  final int minor;
  final int patch;
  final List<String> prerelease;

  const _SemVersion({
    required this.major,
    required this.minor,
    required this.patch,
    required this.prerelease,
  });

  static final RegExp _pattern = RegExp(
    r'^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$',
  );

  static _SemVersion? tryParse(String raw) {
    var normalized = raw.trim();
    if (normalized.startsWith('v')) {
      normalized = normalized.substring(1);
    }
    final plusIndex = normalized.indexOf('+');
    if (plusIndex >= 0) {
      normalized = normalized.substring(0, plusIndex);
    }

    final m = _pattern.firstMatch(normalized);
    if (m == null) return null;
    return _SemVersion(
      major: int.parse(m.group(1)!),
      minor: int.parse(m.group(2)!),
      patch: int.parse(m.group(3)!),
      prerelease: (m.group(4) ?? '')
          .split('.')
          .where((e) => e.isNotEmpty)
          .toList(growable: false),
    );
  }

  bool get hasPrerelease => prerelease.isNotEmpty;

  String? get prereleaseStage {
    if (!hasPrerelease) return null;
    return prerelease.first.toLowerCase();
  }

  bool isRcToBetaMigrationFrom(_SemVersion current) {
    if (major != current.major || minor != current.minor || patch != current.patch) {
      return false;
    }
    return current.prereleaseStage == 'rc' && prereleaseStage == 'beta';
  }

  @override
  int compareTo(_SemVersion other) {
    if (major != other.major) return major.compareTo(other.major);
    if (minor != other.minor) return minor.compareTo(other.minor);
    if (patch != other.patch) return patch.compareTo(other.patch);
    return _comparePrerelease(prerelease, other.prerelease);
  }

  static final RegExp _numeric = RegExp(r'^\d+$');

  static int _comparePrerelease(List<String> a, List<String> b) {
    if (a.isEmpty && b.isEmpty) return 0;
    if (a.isEmpty) return 1;
    if (b.isEmpty) return -1;

    final n = a.length < b.length ? a.length : b.length;
    for (var i = 0; i < n; i++) {
      final ai = a[i];
      final bi = b[i];
      final aNum = _numeric.hasMatch(ai);
      final bNum = _numeric.hasMatch(bi);

      int cmp;
      if (aNum && bNum) {
        cmp = int.parse(ai).compareTo(int.parse(bi));
      } else if (aNum && !bNum) {
        cmp = -1;
      } else if (!aNum && bNum) {
        cmp = 1;
      } else {
        cmp = ai.compareTo(bi);
      }

      if (cmp != 0) return cmp;
    }

    return a.length.compareTo(b.length);
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
