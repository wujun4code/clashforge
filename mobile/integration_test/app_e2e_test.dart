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
import 'dart:io' show HttpClient;
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

      // Keep semantics disabled for the whole integration test run.
      // This avoids sporadic platform-driven semantics toggles that can leave
      // an outstanding SemanticsHandle at end-of-test on emulator runners.
      tester.binding.platformDispatcher.semanticsEnabledTestValue = false;

      await tester.pumpWidget(const ClashForgeApp());
      await tester.pumpAndSettle();

      // ── Round 1: Baseline IP (no VPN) ──────────────────────────────
      _log('Round 1: probing baseline IP (no VPN)');
      final baselineIp = await _probeIp();
      expect(baselineIp, isNotEmpty,
          reason: 'Emulator must have network access before VPN');
      _log('Round 1 baseline: $baselineIp');

      // ── Step 1: Import subscription ─────────────────────────────────
      _log('Navigating to Subscriptions tab');
      await tester.tap(find.text('Subscriptions'));
      await tester.pumpAndSettle();

      _log('Entering subscription URL');
      await tester.enterText(
          find.byKey(const Key('subscription_url_field')), _subUrl);
      await tester.pumpAndSettle();

      // Unfocus URL field so soft keyboard does not obstruct the import button.
      FocusManager.instance.primaryFocus?.unfocus();
      await tester.pumpAndSettle();

      _log('Tapping Import');
      final importBtn = find.byKey(const Key('subscription_import_button'));
      expect(importBtn, findsOneWidget);
      await tester.ensureVisible(importBtn);
      await tester.tap(importBtn, warnIfMissed: false);
      await tester.pump();

      // Wait for fetch + parse (real network; up to 30 s)
      _log('Waiting for subscription fetch…');
      await _waitUntil(
        tester,
        () =>
            find.text('Save').evaluate().isNotEmpty ||
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

      // Dismiss nickname dialog.
      // - autofocus: false in the dialog prevents the soft keyboard from opening
      //   (keyboard would push Save off-screen on the emulator).
      // - Do NOT pumpAndSettle() before tap: the dialog's CircularProgressIndicator
      //   spins forever while _loading=true, causing pumpAndSettle to hang 9 min.
      // - _waitUntil already pumped every 500ms until 'Save' appeared, so the
      //   dialog open animation has long since completed.
      _log('Saving subscription nickname');
      await tester.tap(find.byKey(const Key('save_nickname')));
      // One pump (not pumpAndSettle) to process the tap gesture and run the
      // microtask that resumes _import() after showDialog resolves. The spinner
      // (_loading=true) would cause pumpAndSettle to hang indefinitely, but a
      // single pump is safe — it just renders one frame.
      await tester.pump();

      // Wait for import to complete. Three signals, any one is enough:
      //   1. '已保存' — subscription list header, set by parent setState in
      //      _onNodesImported. Appears even if child setState is delayed.
      //   2. 'Imported'/'nodes' — success banner text from child setState.
      //   3. 'Error:' — error banner; fail fast with the actual message.
      // Uses _hasText (allWidgets, no skipOffstage) instead of find.textContaining
      // because the banner widget can be briefly off-stage while the dialog close
      // animation plays, causing find.textContaining(skipOffstage:true) to miss it.
      await _waitUntil(
        tester,
        () =>
            _hasText(tester, '已保存') ||
            _hasText(tester, 'Imported') ||
            _hasText(tester, 'nodes') ||
            _hasText(tester, 'Error:'),
        timeout: const Duration(seconds: 60),
        label: 'import result',
      );

      _log('Screen after import: ${_visibleTexts(tester)}');

      final importErrFinder = find.textContaining('Error:');
      if (importErrFinder.evaluate().isNotEmpty) {
        fail('[E2E] Import failed: ${_widgetText(importErrFinder)}');
      }
      _log('Subscription imported successfully');

      // ── Step 2: Verify nodes in Proxies tab ─────────────────────────
      _log('Checking Proxies tab');
      await tester.tap(find.text('Routes'));
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
        () =>
            find.text('Connected').evaluate().isNotEmpty ||
            find.textContaining('Error:').evaluate().isNotEmpty ||
            find
                .text('Grant network permission, then tap again')
                .evaluate()
                .isNotEmpty,
        timeout: const Duration(seconds: 30),
        label: 'VPN connect',
      );

      // VPN consent dialog appeared — the background CI adb loop (uiautomator dump + tap)
      // will dismiss it within a few seconds; onActivityResult then starts the VPN service.
      // Wait for the clicker, then tap the toggle again so Flutter gets 'started' → Connected.
      if (find
          .text('Grant network permission, then tap again')
          .evaluate()
          .isNotEmpty) {
        _log(
            '[E2E] VPN consent dialog detected — waiting 15 s for CI clicker to dismiss it…');
        await Future<void>.delayed(const Duration(seconds: 15));
        await tester.pumpAndSettle();

        _log('[E2E] Tapping VPN toggle again after permission grant');
        await tester.tap(find.byKey(const Key('vpn_toggle')));
        await tester.pump();

        await _waitUntil(
          tester,
          () =>
              find.text('Connected').evaluate().isNotEmpty ||
              find.textContaining('Error:').evaluate().isNotEmpty,
          timeout: const Duration(seconds: 30),
          label: 'VPN connect (after permission grant)',
        );
      }

      if (find.textContaining('Error:').evaluate().isNotEmpty) {
        fail(
            '[E2E] VPN connect error: ${_widgetText(find.textContaining("Error:"))}');
      }

      expect(find.text('Connected'), findsOneWidget,
          reason: 'VPN must report Connected');
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
      final mihomoRunning =
          find.textContaining('Running (PID').evaluate().isNotEmpty;
      if (!mihomoRunning) {
        fail(
            '[E2E] mihomo is not running — binary may be incompatible with emulator ABI');
      }
      _log('VPN service: running. Mihomo: running');

      // ── Round 2: IP probe through VPN ───────────────────────────────
      // MUST use mihomo HTTP proxy (127.0.0.1:7890), NOT a direct HTTP request.
      // Reason: the ClashForge app process is excluded from VPN routing via
      // addDisallowedApplication(packageName) to prevent the TUN→mihomo→TUN
      // loopback.  Any direct HTTP request from this test process bypasses the
      // VPN TUN and returns the baseline (Azure runner) IP — not the proxy IP.
      // Loopback (127.0.0.1) also bypasses Android VPN routing, so mihomo port
      // 7890 is reachable directly; mihomo then forwards through the proxy node.
      _log(
          'Round 2: probing IP via mihomo HTTP proxy 127.0.0.1:7890 (up to 5 attempts)…');
      String vpnIp = '';
      for (var attempt = 1; attempt <= 5; attempt++) {
        vpnIp = await _probeIpViaHttpProxy('127.0.0.1', 7890);
        if (vpnIp.isNotEmpty) break;
        _log('Round 2 attempt $attempt — empty, waiting 6 s…');
        await Future<void>.delayed(const Duration(seconds: 6));
        await tester.pumpAndSettle();
      }
      _log('Round 2 VPN IP: $vpnIp');
      expect(vpnIp, isNotEmpty,
          reason: 'Network must be reachable through VPN');
      expect(
        vpnIp,
        isNot(equals(baselineIp)),
        reason:
            'Exit IP must differ from baseline — traffic must route through proxy',
      );
      _log('[PASS] PR-01 Exit IP changed — $baselineIp → $vpnIp');

      // ── PR-01b: Verify exit IP against proxy server ──────────────────────
      // Mirrors OpenWrt probe.sh PR-01: if we know the proxy node's server IP,
      // assert the exit IP matches.  CI uses Tor as exit relay so exit IP ≠
      // proxy server IP — that gets a WARN (not FAIL).
      _log('[E2E] PR-01b: querying mihomo API for active proxy node…');
      final proxyServer = await _queryMihomoProxyServer();
      if (proxyServer.isNotEmpty) {
        final serverIp = await _resolveDoH(proxyServer);
        if (serverIp.isEmpty) {
          _log(
              '[WARN] PR-01b Server IP resolve — DoH lookup failed for $proxyServer');
        } else if (vpnIp == serverIp) {
          _log(
              '[PASS] PR-01b Exit IP matches proxy server — $vpnIp == $serverIp (server: $proxyServer)');
        } else {
          // In CI the exit is a Tor relay, so exit IP ≠ proxy server — expected.
          _log(
              '[WARN] PR-01b Exit IP vs server — $vpnIp ≠ $serverIp (server: $proxyServer); OK if Tor/multi-hop relay');
        }
      } else {
        _log(
            '[WARN] PR-01b Proxy server query — could not read active node from mihomo API at 127.0.0.1:9090');
      }

      // ── PR-02: Target website connectivity via mihomo HTTP proxy ─────────
      // All probes route through 127.0.0.1:7890 (mihomo HTTP proxy) so they
      // work even when TUN fails (packages.xml permission denied on real devices).
      // Mirrors OpenWrt probe.sh PR-02 target list.
      _log(
          '[E2E] PR-02: connectivity probes via mihomo HTTP proxy 127.0.0.1:7890…');
      final pr02Targets = {
        'taobao': 'https://www.taobao.com',
        'music163': 'https://music.163.com',
        'github': 'https://github.com',
        'google': 'https://www.google.com',
      };
      final conn = await _probeConnectivity(pr02Targets);
      var pr02Ok = 0;
      for (final e in conn.entries) {
        final ok = e.value >= 200 && e.value < 400;
        if (ok) pr02Ok++;
        _log('[${ok ? "PASS" : "FAIL"}] PR-02 ${e.key} — HTTP ${e.value}');
      }
      _log('[E2E] PR-02: $pr02Ok/${pr02Targets.length} targets accessible');

      final googleCode = conn['google'] ?? 0;
      expect(
        googleCode >= 200 && googleCode < 400,
        isTrue,
        reason:
            '[E2E] PR-02 FAIL: google.com not reachable via VPN proxy (HTTP $googleCode) — proxy node may be down or routing is broken',
      );

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
      expect(find.text('Tap to connect'), findsOneWidget,
          reason: 'VPN must be off');
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
      await tester.pump(const Duration(milliseconds: 200));
      _drainPendingFlutterExceptions(tester);
    },
    timeout: const Timeout(Duration(minutes: 9)),
    semanticsEnabled: false,
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

// Probes the public IP by routing through mihomo's HTTP proxy via HTTP CONNECT.
// Android VPN with route 0.0.0.0/0 does NOT send loopback traffic through TUN,
// so 127.0.0.1:7890 reaches mihomo directly even when the VPN TUN is broken.
// mihomo then forwards through Tor → different exit IP than Azure baseline.
Future<String> _probeIpViaHttpProxy(String proxyHost, int proxyPort) async {
  for (final endpoint in [
    'https://api.ipify.org?format=json',
    'https://api4.ipify.org?format=json',
    'https://httpbin.org/ip',
  ]) {
    try {
      final httpClient = HttpClient()
        ..findProxy = (uri) => 'PROXY $proxyHost:$proxyPort';
      try {
        final uri = Uri.parse(endpoint);
        final request =
            await httpClient.getUrl(uri).timeout(const Duration(seconds: 15));
        final response =
            await request.close().timeout(const Duration(seconds: 15));
        if (response.statusCode == 200) {
          final body = await response.transform(utf8.decoder).join();
          final json = jsonDecode(body) as Map<String, dynamic>;
          final ip = (json['ip'] ?? json['origin']) as String?;
          if (ip != null && ip.isNotEmpty) return ip.split(',').first.trim();
        }
      } finally {
        httpClient.close();
      }
    } catch (e) {
      _log('Proxy probe $endpoint via $proxyHost:$proxyPort failed: $e');
    }
  }
  return '';
}

String _visibleTexts(WidgetTester tester) => tester.allWidgets
    .whereType<Text>()
    .map((t) => t.data ?? '')
    .where((s) => s.isNotEmpty)
    .take(30)
    .map((s) => '"$s"')
    .join(', ');

// find.textContaining() uses skipOffstage:true by default, which misses widgets
// that are briefly off-stage during the dialog close animation (the banner is set
// in the underlying route while the dialog route is still animating out).
// This helper uses allWidgets (no offstage filter) to match what _visibleTexts sees.
bool _hasText(WidgetTester tester, String substring) => tester.allWidgets
    .whereType<Text>()
    .any((t) => (t.data ?? '').contains(substring));

Future<void> _waitUntil(
  WidgetTester tester,
  bool Function() condition, {
  required Duration timeout,
  required String label,
}) async {
  final deadline = DateTime.now().add(timeout);
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) {
      _log('Timeout $label — visible texts: ${_visibleTexts(tester)}');
      throw Exception(
          '[E2E] Timed out waiting for: $label (${timeout.inSeconds}s)');
    }
    await tester.pump(const Duration(milliseconds: 500));
  }
}

void _drainPendingFlutterExceptions(WidgetTester tester) {
  while (true) {
    final error = tester.takeException();
    if (error == null) return;
    final message = error.toString();
    if (message.contains('SemanticsHandle was active')) {
      _log('[WARN] Suppressed known semantics teardown false-positive');
      continue;
    }
    fail('[E2E] Unexpected Flutter exception: $message');
  }
}

// ── PR-01b / PR-02 helpers ────────────────────────────────────────────────────

// Query mihomo REST API (127.0.0.1:9090/proxies) for the active proxy node's
// server hostname.  Tries common proxy group names from subscription configs.
Future<String> _queryMihomoProxyServer() async {
  try {
    final resp = await http
        .get(Uri.parse('http://127.0.0.1:9090/proxies'))
        .timeout(const Duration(seconds: 5));
    if (resp.statusCode != 200) return '';
    final data = jsonDecode(resp.body) as Map<String, dynamic>;
    final proxies = data['proxies'] as Map<String, dynamic>? ?? {};
    for (final groupName in ['🚀 Proxy', 'Proxy', 'GLOBAL']) {
      final group = proxies[groupName] as Map<String, dynamic>?;
      if (group == null) continue;
      final now = group['now'] as String?;
      if (now == null || now.isEmpty) continue;
      final node = proxies[now] as Map<String, dynamic>?;
      final server = node?['server'] as String?;
      if (server != null && server.isNotEmpty) {
        _log('Active proxy node: $now → server=$server');
        return server;
      }
    }
  } catch (e) {
    _log('queryMihomoProxyServer: $e');
  }
  return '';
}

// Resolve a hostname to its first IPv4 A record via Google DNS-over-HTTPS.
// Routes through mihomo HTTP proxy (127.0.0.1:7890) so it works whether
// TUN is active or not (loopback bypasses Android VPN routing entirely).
Future<String> _resolveDoH(String hostname) async {
  final httpClient = HttpClient()..findProxy = (_) => 'PROXY 127.0.0.1:7890';
  try {
    final uri = Uri.parse(
        'https://dns.google/resolve?name=${Uri.encodeComponent(hostname)}&type=A');
    final request =
        await httpClient.getUrl(uri).timeout(const Duration(seconds: 8));
    final response = await request.close().timeout(const Duration(seconds: 8));
    if (response.statusCode == 200) {
      final body = await response.transform(utf8.decoder).join();
      final data = jsonDecode(body) as Map<String, dynamic>;
      final answers = (data['Answer'] as List<dynamic>?) ?? [];
      for (final a in answers) {
        final record = a as Map<String, dynamic>;
        if (record['type'] == 1) {
          final ip = record['data'] as String? ?? '';
          if (ip.isNotEmpty) return ip;
        }
      }
    }
  } catch (e) {
    _log('resolveDoH $hostname: $e');
  } finally {
    httpClient.close();
  }
  return '';
}

// Probe a map of named URLs via mihomo HTTP proxy (127.0.0.1:7890).
// Returns name → HTTP status code; 0 means connection failed entirely.
// Uses GET + drain to avoid full body download; follows redirects by default.
Future<Map<String, int>> _probeConnectivity(Map<String, String> targets) async {
  final results = <String, int>{};
  for (final entry in targets.entries) {
    final name = entry.key;
    final url = entry.value;
    var code = 0;
    final httpClient = HttpClient()..findProxy = (_) => 'PROXY 127.0.0.1:7890';
    try {
      final request = await httpClient
          .getUrl(Uri.parse(url))
          .timeout(const Duration(seconds: 12));
      final response =
          await request.close().timeout(const Duration(seconds: 12));
      code = response.statusCode;
      await response.drain<void>();
    } catch (e) {
      _log('probeConnectivity $name: $e');
    } finally {
      httpClient.close();
    }
    results[name] = code;
  }
  return results;
}
