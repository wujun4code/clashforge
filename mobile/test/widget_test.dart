import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/material.dart';
import 'package:clashforge_mobile/main.dart';

void main() {
  testWidgets('ClashForgeApp layout test', (WidgetTester tester) async {
    await tester.pumpWidget(const ClashForgeApp());

    expect(find.text('ClashForge'), findsOneWidget);
    expect(find.byIcon(Icons.shield_outlined), findsOneWidget);
    expect(find.text('Tap to connect'), findsOneWidget);
    expect(find.text('No node selected'), findsOneWidget);

    // Navigation bar tabs
    expect(find.text('Home'), findsOneWidget);
    expect(find.text('Proxies'), findsOneWidget);
    expect(find.text('Subscriptions'), findsOneWidget);
    expect(find.text('Logs'), findsOneWidget);
  });
}
