import 'package:clashforge_mobile/l10n/app_localizations.dart';
import 'package:clashforge_mobile/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../integration_test/settings_about_navigation.dart';

void main() {
  testWidgets('real Settings About navigation at 400x640 with bottom navigation',
      (tester) async {
    tester.view.physicalSize = const Size(400, 640);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    SharedPreferences.setMockInitialValues({});
    const channel = MethodChannel('com.clashforge.mobile/vpn');
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    var systemInfoCalls = 0;
    messenger.setMockMethodCallHandler(channel, (call) async {
      switch (call.method) {
        case 'getSystemInfo':
          systemInfoCalls++;
          // Navigation-only fixture: no simulated successful VPN/core.
          return <String, dynamic>{};
        case 'listInstalledApps':
          return <dynamic>[];
        default:
          return 'ok';
      }
    });
    addTearDown(() => messenger.setMockMethodCallHandler(channel, null));
    await tester.pumpWidget(MaterialApp(
      locale: const Locale('en'),
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
    expect(find.text('Home').hitTestable(), findsOneWidget);
    expect(find.text('Runtime Profile'), findsOneWidget);
    expect(find.text('App Routing'), findsOneWidget);
    final about = find.text('About');
    expect(about, findsOneWidget);
    expect(about.hitTestable(), findsNothing,
        reason: 'The original direct tap is outside the Settings viewport');
    final scrollable = find.ancestor(
        of: about, matching: find.byType(Scrollable)).first;
    final position = tester.state<ScrollableState>(scrollable).position;
    final beforeOffset = position.pixels;
    final beforeCalls = systemInfoCalls;
    await openSettingsAbout(tester);
    expect(position.pixels, greaterThan(beforeOffset));
    expect(systemInfoCalls, greaterThan(beforeCalls),
        reason: 'The actual About route must request its diagnostics');
    expect(find.text('Nodes loaded'), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pumpAndSettle();
  });
}
