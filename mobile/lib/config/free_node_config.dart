import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';
import 'package:encrypt/encrypt.dart' as enc;

class FreeNodeConfig {
  static const _cipher = String.fromEnvironment('FREE_NODE_CIPHER');
  static const _rawKey = String.fromEnvironment('FREE_NODE_KEY');

  // Decrypt the subscription URL embedded at build time (AES-256-CBC).
  // Key derivation: UTF-8 bytes of _rawKey truncated/padded to 32 bytes.
  static String? get subscriptionUrl {
    if (_cipher.isEmpty || _rawKey.isEmpty) return null;
    try {
      final combined = base64Decode(_cipher);
      if (combined.length < 17) return null;
      final iv = enc.IV(Uint8List.fromList(combined.sublist(0, 16)));
      final keyBytes = utf8.encode(_rawKey);
      final key32 = Uint8List(32)
        ..setRange(0, min(keyBytes.length, 32), keyBytes);
      final key = enc.Key(key32);
      final encrypter = enc.Encrypter(enc.AES(key, mode: enc.AESMode.cbc));
      final encrypted = enc.Encrypted(Uint8List.fromList(combined.sublist(16)));
      return encrypter.decrypt(encrypted, iv: iv);
    } catch (_) {
      return null;
    }
  }

  // Raw 32-byte AES key decoded from the hex string _rawKey.
  // Used to decrypt the subscription response (AES-256-GCM).
  static Uint8List? get rawKeyBytes {
    if (_rawKey.length != 64) return null;
    try {
      final bytes = Uint8List(32);
      for (var i = 0; i < 64; i += 2) {
        bytes[i ~/ 2] = int.parse(_rawKey.substring(i, i + 2), radix: 16);
      }
      return bytes;
    } catch (_) {
      return null;
    }
  }
}
