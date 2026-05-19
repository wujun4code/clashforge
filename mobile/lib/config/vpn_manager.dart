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
}
