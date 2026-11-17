import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// Shared by the device E2E and the constrained real-Settings regression.
Future<void> openSettingsAbout(WidgetTester tester) async {
  final about = find.text('About');
  expect(about, findsOneWidget);
  final aboutRow =
      find.ancestor(of: about, matching: find.byType(InkWell)).first;
  await tester.ensureVisible(aboutRow);
  await tester.pumpAndSettle();
  expect(about.hitTestable(), findsOneWidget,
      reason: 'About navigation target must be visible and hit-testable');
  await tester.tap(about);
  await tester.pumpAndSettle();
  expect(find.byIcon(Icons.arrow_back).hitTestable(), findsOneWidget,
      reason: 'About route must be reached before checking native status');
  expect(find.text('Nodes loaded'), findsOneWidget);
}
