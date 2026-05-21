// E2E Integration Test — ClashForge Mobile
//
// Mirrors the OpenWrt E2E 3-round pattern:
//   Round 1  — Baseline IP (no VPN)
//   Import subscription → select first node → connect VPN
//   Round 2  — IP probe through VPN (must differ from baseline)
//   Disconnect VPN
//   Round 3  — IP probe restored (must match baseline)
//
// Required: --dart-define=SUBSCRIPTION_URL=<url>
// CI: a background adb loop (uiautomator dump + tap) auto-dismisses the VPN consent dialog.

import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:http/http.dart' as http;
import 'package:clashforge_mobile/main.dart';

const _subUrl = String.fromEnvironment('SUBSCRIPTION_URL');

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
    'E2E: subscription import → VPN on (Round 2 probe) → VPN off (Round 3 restore)',
    (tester) async {
      if (_subUrl.isEmpty) {
        markTestSkipped('SUBSCRIPTION_URL not set — skipping E2E');
        return;
      }

      await tester.pumpWidget(const ClashForgeApp());
      await tester.pumpAndSettle();

      // ── Round 1: Baseline IP (no VPN) ──────────────────────────────
      _log('Round 1: probing baseline IP (no VPN)');
      final baselineIp = await _probeIp();
      expect(baselineIp, isNotEmpty, reason: 'Emulator must have network access before VPN');
      _log('Round 1 baseline: $baselineIp');

      // ── Step 1: Import subscription ─────────────────────────────────
      _log('Navigating to Subscriptions tab');
      await tester.tap(find.text('Subscriptions'));
      await tester.pumpAndSettle();

      _log('Entering subscription URL');
      await tester.enterText(find.byKey(const Key('subscription_url_field')), _subUrl);
      await tester.pumpAndSettle();

      _log('Tapping Import');
      await tester.tap(find.text('Import'));
      await tester.pump();

      // Wait for fetch + parse (real network; up to 30 s)
      _log('Waiting for subscription fetch…');
      await _waitUntil(
        tester,
        () => find.text('Save').evaluate().isNotEmpty ||
              find.textContaining('Error').evaluate().isNotEmpty ||
              find.textContaining('failed').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 30),
        label: 'subscription fetch',
      );

      final errFinder = find.textContaining('Error');
      final failFinder = find.textContaining('failed');
      if (errFinder.evaluate().isNotEmpty || failFinder.evaluate().isNotEmpty) {
        final msg = errFinder.evaluate().isNotEmpty
            ? _widgetText(errFinder)
            : _widgetText(failFinder);
        fail('[E2E] Subscription fetch failed: $msg');
      }

      // Dismiss nickname dialog with default name
      _log('Saving subscription nickname');
      await tester.tap(find.text('Save'));
      await tester.pumpAndSettle();

      // Verify import success banner ("Imported N nodes as …")
      expect(
        find.textContaining('nodes').evaluate().isNotEmpty ||
            find.textContaining('Imported').evaluate().isNotEmpty,
        isTrue,
        reason: 'Import success banner must appear',
      );
      _log('Subscription imported successfully');

      // ── Step 2: Verify nodes in Proxies tab ─────────────────────────
      _log('Checking Proxies tab');
      await tester.tap(find.text('Proxies'));
      await tester.pumpAndSettle();
      expect(
        find.text('No nodes yet').evaluate().isEmpty,
        isTrue,
        reason: 'Proxy list must be non-empty after import',
      );

      // ── Step 3: Connect VPN ─────────────────────────────────────────
      _log('Navigating to Home tab');
      await tester.tap(find.text('Home'));
      await tester.pumpAndSettle();

      _log('Tapping VPN toggle (connect)');
      await tester.tap(find.byKey(const Key('vpn_toggle')));
      await tester.pump();

      _log('Waiting for VPN to connect…');
      await _waitUntil(
        tester,
        () => find.text('Connected').evaluate().isNotEmpty ||
              find.textContaining('Error:').evaluate().isNotEmpty ||
              find.text('Grant VPN permission, then tap again').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 30),
        label: 'VPN connect',
      );

      // VPN consent dialog appeared — the background CI adb loop (uiautomator dump + tap)
      // will dismiss it within a few seconds; onActivityResult then starts the VPN service.
      // Wait for the clicker, then tap the toggle again so Flutter gets 'started' → Connected.
      if (find.text('Grant VPN permission, then tap again').evaluate().isNotEmpty) {
        _log('[E2E] VPN consent dialog detected — waiting 15 s for CI clicker to dismiss it…');
        await Future<void>.delayed(const Duration(seconds: 15));
        await tester.pumpAndSettle();

        _log('[E2E] Tapping VPN toggle again after permission grant');
        await tester.tap(find.byKey(const Key('vpn_toggle')));
        await tester.pump();

        await _waitUntil(
          tester,
          () => find.text('Connected').evaluate().isNotEmpty ||
                find.textContaining('Error:').evaluate().isNotEmpty,
          timeout: const Duration(seconds: 30),
          label: 'VPN connect (after permission grant)',
        );
      }

      if (find.textContaining('Error:').evaluate().isNotEmpty) {
        fail('[E2E] VPN connect error: ${_widgetText(find.textContaining("Error:"))}');
      }

      expect(find.text('Connected'), findsOneWidget, reason: 'VPN must report Connected');
      _log('VPN connected (UI)');

      // Give mihomo time to fully start and establish proxy connections
      _log('Waiting 15 s for mihomo to establish connections…');
      await Future<void>.delayed(const Duration(seconds: 15));
      await tester.pumpAndSettle();

      // ── Step 4: Verify VPN + mihomo running (About screen) ──────────
      _log('Checking About screen for runtime status');
      await tester.tap(find.text('Settings'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('About'));
      await tester.pumpAndSettle();
      await Future<void>.delayed(const Duration(seconds: 2));
      await tester.pumpAndSettle();

      // VPN row
      expect(
        find.textContaining('Running').evaluate().isNotEmpty,
        isTrue,
        reason: 'About screen must show at least one Running service',
      );

      // Mihomo specifically — text is "Running (PID …)"
      final mihomoRunning = find.textContaining('Running (PID').evaluate().isNotEmpty;
      if (!mihomoRunning) {
        fail('[E2E] mihomo is not running — binary may be incompatible with emulator ABI');
      }
      _log('VPN service: running. Mihomo: running');

      // ── Round 2: IP probe through VPN ───────────────────────────────
      // Retry up to 5 times with 6 s gaps — Tor relays can take several seconds to
      // establish the first circuit, and mihomo's url-test needs time to settle.
      _log('Round 2: probing IP through VPN (up to 5 attempts)…');
      String vpnIp = '';
      for (var attempt = 1; attempt <= 5; attempt++) {
        vpnIp = await _probeIp();
        if (vpnIp.isNotEmpty) break;
        _log('Round 2 attempt $attempt returned empty — waiting 6 s…');
        await Future<void>.delayed(const Duration(seconds: 6));
        await tester.pumpAndSettle();
      }
      _log('Round 2 VPN IP: $vpnIp');
      expect(vpnIp, isNotEmpty, reason: 'Network must be reachable through VPN');
      expect(
        vpnIp,
        isNot(equals(baselineIp)),
        reason: 'Exit IP must differ from baseline — traffic must route through proxy',
      );
      _log('Round 2 ✓ IP changed ($baselineIp → $vpnIp)');

      // ── Step 5: Disconnect VPN ──────────────────────────────────────
      _log('Navigating back to Home');
      await tester.tap(find.byIcon(Icons.arrow_back));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Home'));
      await tester.pumpAndSettle();

      _log('Tapping VPN toggle (disconnect)');
      await tester.tap(find.byKey(const Key('vpn_toggle')));
      await tester.pump();

      _log('Waiting for VPN to disconnect…');
      await _waitUntil(
        tester,
        () => find.text('Tap to connect').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 20),
        label: 'VPN disconnect',
      );
      expect(find.text('Tap to connect'), findsOneWidget, reason: 'VPN must be off');
      _log('VPN disconnected (UI)');

      // Allow kernel to release the TUN fd and clear routes
      await Future<void>.delayed(const Duration(seconds: 5));

      // ── Round 3: IP probe — must restore to baseline ─────────────────
      _log('Round 3: probing IP after VPN off…');
      final restoredIp = await _probeIp();
      _log('Round 3 restored IP: $restoredIp');
      expect(
        restoredIp,
        equals(baselineIp),
        reason: 'Exit IP must return to baseline after VPN is stopped',
      );
      _log('Round 3 ✓ IP restored ($restoredIp == $baselineIp)');

      _log('=== E2E COMPLETE ✓ ===');
    },
    timeout: const Timeout(Duration(minutes: 5)),
  );
}

// ── Helpers ──────────────────────────────────────────────────────────────────

void _log(String msg) => debugPrint('[E2E] $msg');

String _widgetText(Finder f) {
  try {
    final w = f.evaluate().first.widget;
    if (w is Text) return w.data ?? '';
  } catch (_) {}
  return '(unknown)';
}

Future<String> _probeIp() async {
  for (final endpoint in [
    'https://api.ipify.org?format=json',
    'https://api4.ipify.org?format=json',
    'https://httpbin.org/ip',
  ]) {
    try {
      final resp = await http
          .get(Uri.parse(endpoint))
          .timeout(const Duration(seconds: 15));
      if (resp.statusCode == 200) {
        final body = jsonDecode(resp.body) as Map<String, dynamic>;
        final ip = (body['ip'] ?? body['origin']) as String?;
        if (ip != null && ip.isNotEmpty) return ip.split(',').first.trim();
      }
    } catch (e) {
      _log('Probe $endpoint failed: $e');
    }
  }
  return '';
}

Future<void> _waitUntil(
  WidgetTester tester,
  bool Function() condition, {
  required Duration timeout,
  required String label,
}) async {
  final deadline = DateTime.now().add(timeout);
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) {
      throw Exception('[E2E] Timed out waiting for: $label (${timeout.inSeconds}s)');
    }
    await tester.pump(const Duration(milliseconds: 500));
  }
}
