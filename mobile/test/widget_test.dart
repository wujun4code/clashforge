import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/material.dart';
import 'package:clashforge_mobile/main.dart';

void main() {
  testWidgets('ClashForgeApp layout test', (WidgetTester tester) async {
    await tester.pumpWidget(const ClashForgeApp());

    // Verify title and main widgets are present
    expect(find.text('ClashForge Mobile'), findsOneWidget);
    expect(find.byIcon(Icons.power_settings_new), findsOneWidget);
    expect(find.text('Disconnected'), findsOneWidget);
    expect(find.text('No proxy nodes loaded.'), findsOneWidget);

    // Verify Import button is present
    expect(find.text('Import'), findsOneWidget);
  });
}
