// Shared ordering contract. Recording-fake tests are not Android/device evidence.
import 'dart:async';

typedef NativeControl = Future<String> Function();

/// Uses existing persisted config/permission. No delay manufactures the race:
/// START and STOP are dispatched back-to-back before either is awaited.
Future<void> assertNativeRapidReconnect({
  required NativeControl start,
  required NativeControl stop,
  required Future<Map<String, dynamic>> Function() systemInfo,
  required Future<void> Function() assertCoreReady,
  Duration timeout = const Duration(seconds: 25),
}) async {
  Future<T> bounded<T>(Future<T> Function() action) =>
      Future<T>.sync(action).timeout(timeout);

  void require(bool value, String phase) {
    // Never include native errors, config, URLs or credentials in public logs.
    if (!value) throw StateError('Native reconnect contract failed: $phase');
  }

  Future<void> assertOff() async {
    final info = await bounded(systemInfo);
    require(info['vpn_running'] == false && info['mihomo_running'] == false,
        'service must be off');
  }

  await assertOff();
  final starting = bounded(start);
  final stopping = bounded(stop);
  // Observe BOTH completions, including an error from either future. A pending
  // STOP must never be bypassed by a replacement START.
  final results = await Future.wait([starting, stopping]);
  final first = results[0];
  require(
      first == 'started' ||
          first == 'error:VPN startup superseded' ||
          (first.startsWith('error:') &&
              first.toLowerCase().contains('cancel')),
      'initial START must complete or cancel');
  require(results[1] == 'stopped', 'initial STOP must complete');
  await assertOff();

  require(await bounded(start) == 'started', 'replacement START must complete');
  try {
    final info = await bounded(systemInfo);
    require(
        info['vpn_running'] == true &&
            info['mihomo_running'] == true &&
            info['mihomo_pid'] is num &&
            (info['mihomo_pid'] as num) > 0,
        'replacement must own a running core');
    await bounded(assertCoreReady);
  } finally {
    require(await bounded(stop) == 'stopped', 'final STOP must complete');
    await assertOff();
  }
}
