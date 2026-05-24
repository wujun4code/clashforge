import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:clashforge_mobile/main.dart';

void main() {
  testWidgets('ClashForgeApp layout test', (WidgetTester tester) async {
    SharedPreferences.setMockInitialValues({});
    await tester.pumpWidget(const ClashForgeApp());
    // Extra pump so localization delegates finish loading before find.text calls.
    await tester.pump();

    expect(find.text('ClashForge'), findsOneWidget);
    expect(find.byIcon(Icons.power_settings_new), findsOneWidget);
    expect(find.text('Tap to connect'), findsOneWidget);
    expect(find.text('Home'), findsOneWidget);
    expect(find.text('Routes'), findsOneWidget);
    expect(find.text('Subscriptions'), findsOneWidget);
    expect(find.text('Settings'), findsOneWidget);
  });
}
