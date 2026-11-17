import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:clashforge_mobile/main.dart';
import 'package:clashforge_mobile/config/app_routing_controller.dart';
import 'package:clashforge_mobile/config/vpn_manager.dart';
import 'package:clashforge_mobile/l10n/app_localizations.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('com.clashforge.mobile/vpn');
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  final previous = AppRoutingSetting('all', {});
  final desired = AppRoutingSetting('bypass', {'org.test.app'});
  late AppRoutingSetting native;
  late List<String> calls;
  late bool failNative;
  late bool failPersistence;
  late bool restartSuccess;
  late AppRoutingController controller;
  setUp(() {
    SharedPreferences.setMockInitialValues({'routing_mode': 'all'});
    native = previous;
    calls = [];
    failNative = false;
    failPersistence = false;
    restartSuccess = true;
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call.method);
      if (call.method == 'listInstalledApps') return <dynamic>[];
      if (call.method == 'setAppRouting') {
        final args = Map<String, dynamic>.from(call.arguments as Map);
        if (failNative && args['mode'] == 'bypass') {
          throw PlatformException(
              code: 'failed', message: 'native setter failed');
        }
        native = AppRoutingSetting(
            args['mode'] as String, (args['packages'] as List).cast<String>());
      }
      return 'ok';
    });
    controller = AppRoutingController(
      setNative: (setting) =>
          VpnManager.setAppRouting(setting.mode, setting.packages),
      persist: (setting) async {
        final prefs = await SharedPreferences.getInstance();
        // Simulate partial write followed by persistence failure.
        await prefs.setString('routing_mode', setting.mode);
        if (failPersistence && setting.mode == 'bypass') {
          throw StateError('disk full');
        }
        await prefs.setStringList(
            'routing_packages', setting.packages.toList());
      },
      restart: () async {
        calls.add('restart');
        return AppRoutingResult(
            success: restartSuccess,
            message: 'restart failed: permission needed');
      },
    );
  });
  tearDown(() => messenger.setMockMethodCallHandler(channel, null));

  for (final failure in ['native', 'persistence', 'restart']) {
    test('$failure failure rolls native and persistent desired state back',
        () async {
      failNative = failure == 'native';
      failPersistence = failure == 'persistence';
      restartSuccess = failure != 'restart';
      final result = await controller.apply(
          previous: previous, desired: desired, connected: true);
      expect(result.success, isFalse);
      expect(native.mode, 'all');
      expect((await SharedPreferences.getInstance()).getString('routing_mode'),
          'all');
      expect(calls.where((s) => s == 'restart').length,
          failure == 'restart' ? 1 : 0);
    });
  }
  test(
      'no-node preflight preserves existing connection control without native changes',
      () async {
    final result = await controller.apply(
        previous: previous,
        desired: desired,
        connected: true,
        preflightError: '重启失败：当前无可用节点');
    expect(result.success, isFalse);
    expect(result.message, contains('无可用节点'));
    expect(calls, isEmpty);
  });
  for (final connected in [false, true]) {
    test('success connected=$connected commits native and preferences',
        () async {
      final result = await controller.apply(
          previous: previous, desired: desired, connected: connected);
      expect(result.success, isTrue);
      expect(native.mode, 'bypass');
      expect((await SharedPreferences.getInstance()).getString('routing_mode'),
          'bypass');
      expect(calls.contains('restart'), connected);
    });
  }

  test('rollback failure warns and empty allowlist performs no writes',
      () async {
    final rejected = await controller.apply(
        previous: previous,
        desired: AppRoutingSetting('allowlist', {}),
        connected: true);
    expect(rejected.success, isFalse);
    expect(rejected.message, contains('请选择至少一个应用'));
    expect(calls, isEmpty);
    final rollback = AppRoutingController(
      setNative: (setting) async =>
          setting.mode == 'all' ? 'error: native rollback failed' : 'ok',
      persist: (setting) async {
        if (setting.mode == 'all') throw StateError('prefs rollback failed');
      },
      restart: () async => const AppRoutingResult(
          success: false, message: 'error: real start failed'),
    );
    final result = await rollback.apply(
        previous: previous, desired: desired, connected: true);
    expect(result.success, isFalse);
    expect(result.message, contains('real start failed'));
    expect(result.message, contains('恢复旧设置失败'));
    expect(result.message, contains('native rollback failed'));
    expect(result.message, contains('prefs rollback failed'));
  });

  for (final outcome in [
    'stop failure',
    'permission needed',
    'success',
    'pending stop failure',
    'pending start failure',
  ]) {
    testWidgets(
        'HomeScreen connected restart $outcome preserves correct controls',
        (tester) async {
      final stopFails = outcome.contains('stop failure');
      final pending = outcome.startsWith('pending');
      final nativeCompletion = Completer<String>();
      final routingModes = <String>[];
      SharedPreferences.setMockInitialValues({
        'cf_active_sub_id_v1': 'test',
        'cf_subscriptions_v2': [
          jsonEncode({
            'id': 'test',
            'nickname': 'test',
            'url': '',
            'custom_rules': ['MATCH,DIRECT'],
            'nodes': [
              {
                'name': 'test',
                'type': 'ss',
                'server': '192.0.2.1',
                'port': 443,
                'cipher': 'aes-128-gcm',
                'password': 'synthetic-test-only'
              }
            ],
          })
        ],
      });
      final suspendedDiagnostics = Completer<Map<dynamic, dynamic>>();
      var starts = 0;
      messenger.setMockMethodCallHandler(channel, (call) async {
        calls.add(call.method);
        switch (call.method) {
          case 'getSystemInfo':
            return suspendedDiagnostics.future;
          case 'listInstalledApps':
            return <dynamic>[];
          case 'getFilesDir':
            return '/synthetic/files';
          case 'setAppRouting':
            routingModes.add((call.arguments as Map)['mode'] as String);
            return 'ok';
          case 'startVpn':
            starts++;
            if (starts > 1 && outcome == 'pending start failure') {
              return nativeCompletion.future;
            }
            return starts == 1 || outcome == 'success'
                ? 'started'
                : 'permission_needed';
          case 'stopVpn':
            if (outcome == 'pending stop failure') {
              return nativeCompletion.future;
            }
            return stopFails ? 'error: stop rejected' : 'stopped';
          default:
            return 'ok';
        }
      });
      await tester.pumpWidget(MaterialApp(
        theme: ThemeData(
            platform: TargetPlatform.android,
            splashFactory: InkRipple.splashFactory),
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: HomeScreen(onLocaleChanged: (_) {}),
      ));
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.power_settings_new));
      await tester.pumpAndSettle();
      expect(find.text('Disconnect'), findsOneWidget);
      await tester.tap(find.text('Settings'));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('App Routing'));
      await tester.tap(find.text('App Routing'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Bypass Selected Apps'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Save'));
      if (pending) {
        // An unresolved native completion must not close or claim success.
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 100));
        expect(find.byType(AppRoutingSheet), findsOneWidget);
        expect(find.text('分应用设置已生效'), findsNothing);
        expect(starts, stopFails ? 1 : 2);
        expect(calls.where((call) => call == 'stopVpn'), hasLength(1));
        nativeCompletion.complete(
            stopFails ? 'error: stop rejected' : 'error: real start failed');
      }
      await tester.pumpAndSettle();
      expect(find.byType(AppRoutingSheet),
          outcome == 'success' ? findsNothing : findsOneWidget);
      if (outcome != 'success') {
        expect(
            find.textContaining(stopFails
                ? 'stop rejected'
                : outcome == 'pending start failure'
                    ? 'real start failed'
                    : '需要系统授权'),
            findsOneWidget);
      }
      expect(
          (await SharedPreferences.getInstance()).getString('vpn_routing_mode'),
          outcome == 'success' ? 'bypass' : 'all');
      expect(starts, stopFails ? 1 : 2);
      if (pending) {
        expect(routingModes.last, 'all');
        expect(
            (await SharedPreferences.getInstance())
                .getStringList('vpn_routing_packages'),
            isEmpty);
      }
      // Dismiss explicitly, not as a consequence of the failed save.
      if (outcome != 'success') {
        Navigator.of(tester.element(find.byType(AppRoutingSheet))).pop();
      }
      await tester.pumpAndSettle();
      await tester.tap(find.text('Home'));
      await tester.pumpAndSettle();
      expect(
          find.text('Disconnect'),
          outcome == 'permission needed' || outcome == 'pending start failure'
              ? findsNothing
              : findsOneWidget);
      if (pending) {
        // Reopen from HomeScreen to verify its committed setting stayed old.
        await tester.tap(find.text('Settings'));
        await tester.pumpAndSettle();
        await tester.ensureVisible(find.text('App Routing'));
        await tester.tap(find.text('App Routing'));
        await tester.pumpAndSettle();
        expect(find.byType(AppRoutingSheet), findsOneWidget);
        expect(
            tester
                .widget<AppRoutingSheet>(find.byType(AppRoutingSheet))
                .initialMode,
            'all');
      }
      await tester.pumpWidget(const SizedBox());
      await tester.pumpAndSettle();
    });
  }

  testWidgets(
      'HomeScreen settings callback rolls back native failure then saves disconnected',
      (tester) async {
    failNative = true;
    await tester.pumpWidget(MaterialApp(
      theme: ThemeData(
          platform: TargetPlatform.android,
          splashFactory: InkRipple.splashFactory),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: HomeScreen(onLocaleChanged: (_) {}),
    ));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('App Routing'));
    await tester.tap(find.text('App Routing'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Bypass Selected Apps'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();
    expect(find.byType(AppRoutingSheet), findsOneWidget);
    expect(find.textContaining('native setter failed'), findsOneWidget);
    expect(
        (await SharedPreferences.getInstance()).getString('vpn_routing_mode'),
        'all');
    failNative = false;
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();
    expect(find.byType(AppRoutingSheet), findsNothing);
    expect(
        (await SharedPreferences.getInstance()).getString('vpn_routing_mode'),
        'bypass');
    expect(calls.contains('startVpn'), isFalse);
    expect(calls.contains('stopVpn'), isFalse);
  });

  testWidgets(
      'actual routing sheet keeps failed restart visible and allows successful retry',
      (tester) async {
    restartSuccess = false;
    await tester.pumpWidget(MaterialApp(
      theme: ThemeData(splashFactory: InkRipple.splashFactory),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: Builder(
          builder: (context) => Scaffold(
              body: TextButton(
                  onPressed: () => showModalBottomSheet(
                      context: context,
                      isScrollControlled: true,
                      builder: (_) => AppRoutingSheet(
                          initialMode: 'bypass',
                          initialPackages: desired.packages,
                          onSave: (mode, packages) => controller.apply(
                              previous: previous,
                              desired: AppRoutingSetting(mode, packages),
                              connected: true))),
                  child: const Text('open')))),
    ));
    await tester.pumpAndSettle();
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();
    expect(find.byType(AppRoutingSheet), findsOneWidget);
    expect(find.textContaining('restart failed'), findsOneWidget);
    expect(native.mode, 'all');
    restartSuccess = true;
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();
    expect(find.byType(AppRoutingSheet), findsNothing);
    expect(native.mode, 'bypass');
  });
}
