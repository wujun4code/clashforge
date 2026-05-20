import 'package:flutter/services.dart';

class VpnManager {
  static const MethodChannel _channel = MethodChannel('com.clashforge.mobile/vpn');

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
      final String result = await _channel.invokeMethod('writeConfig', {'yaml': yaml});
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
}
