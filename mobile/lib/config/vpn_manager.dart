import 'package:flutter/services.dart';

class VpnAppInfo {
  const VpnAppInfo({required this.packageName, required this.label});

  final String packageName;
  final String label;
}

class VpnManager {
  static const MethodChannel _channel =
      MethodChannel('com.clashforge.mobile/vpn');

  static Future<String> startVpn() async {
    try {
      final String result = await _channel.invokeMethod('startVpn');
      return result;
    } on PlatformException catch (e) {
      return 'error: ${e.message}';
    }
  }

  static Future<String> stopVpn() async {
    try {
      final String result = await _channel.invokeMethod('stopVpn');
      return result;
    } on PlatformException catch (e) {
      return 'error: ${e.message}';
    }
  }

  static Future<String> getFilesDir() async {
    try {
      final String result = await _channel.invokeMethod('getFilesDir');
      return result;
    } on PlatformException catch (e) {
      return 'error: ${e.message}';
    }
  }

  static Future<String> writeConfig(String yaml) async {
    try {
      final String result =
          await _channel.invokeMethod('writeConfig', {'yaml': yaml});
      return result;
    } on PlatformException catch (e) {
      return 'error: ${e.message}';
    }
  }

  static Future<Map<String, dynamic>> getSystemInfo() async {
    try {
      final result = await _channel.invokeMethod<Map>('getSystemInfo');
      return Map<String, dynamic>.from(result ?? {});
    } on PlatformException catch (e) {
      return {'error': e.message};
    }
  }

  static Future<String> readNativeLogs() async {
    try {
      final String result = await _channel.invokeMethod('readNativeLogs');
      return result;
    } on PlatformException catch (e) {
      return 'error: ${e.message}';
    }
  }

  static Future<String> clearNativeLogs() async {
    try {
      final String result = await _channel.invokeMethod('clearNativeLogs');
      return result;
    } on PlatformException catch (e) {
      return 'error: ${e.message}';
    }
  }

  static Future<List<VpnAppInfo>> listInstalledApps() async {
    try {
      final result = await _channel.invokeMethod<List>('listInstalledApps');
      return (result ?? const [])
          .whereType<Map>()
          .map((item) => Map<String, dynamic>.from(item))
          .map((item) => VpnAppInfo(
                packageName: (item['package'] ?? '').toString(),
                label: (item['label'] ?? item['package'] ?? '').toString(),
              ))
          .where((item) => item.packageName.isNotEmpty)
          .toList(growable: false);
    } on PlatformException {
      return const [];
    }
  }

  static Future<String> setAppRouting(
    String mode,
    Iterable<String> packages,
  ) async {
    try {
      final String result = await _channel.invokeMethod('setAppRouting', {
        'mode': mode,
        'packages': packages.toList(growable: false),
      });
      return result;
    } on PlatformException catch (e) {
      return 'error: ${e.message}';
    }
  }
}
