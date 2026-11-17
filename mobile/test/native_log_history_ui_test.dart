import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:clashforge_mobile/main.dart';
import 'package:clashforge_mobile/logger/app_logger.dart';
import 'package:clashforge_mobile/logger/native_log_history.dart';
import 'package:clashforge_mobile/l10n/app_localizations.dart';

Widget host() => MaterialApp(
      theme: ThemeData(splashFactory: InkRipple.splashFactory),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: HomeScreen(onLocaleChanged: (_) {}),
    );

const vpn = MethodChannel('com.clashforge.mobile/vpn');
const events = MethodChannel('com.clashforge.mobile/logs');
String record(String id, int ts, String message) => jsonEncode({
      'ts': ts,
      'level': 'info',
      'component': 'vpn',
      'message': message,
      'fields': {'_nativeEventId': id, 'count': 1},
    });

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    AppLogger.instance.clear();
    messenger.setMockMethodCallHandler(events, (_) async => null);
  });
  tearDown(() {
    messenger.setMockMethodCallHandler(vpn, null);
    messenger.setMockMethodCallHandler(events, null);
  });

  test('legacy identical live occurrences are not collapsed', () {
    final history = NativeLogHistory();
    final legacy = jsonEncode(
        {'ts': 1, 'level': 'info', 'component': 'vpn', 'message': 'repeat'});
    history.addLive(legacy);
    history.addLive(legacy);
    expect(history.entries.length, 2);
  });

  test('snapshot order wins equal timestamp live/history ties', () {
    final history = NativeLogHistory();
    history.addLive(record('b', 1, 'second'));
    history.addHistory(
        [record('a', 1, 'first'), record('b', 1, 'second')].join('\n'));
    expect(history.entries.map((e) => e.message), ['first', 'second']);
  });

  test('bounded history retains equal repeats and timestamp order', () {
    final history = NativeLogHistory(capacity: 3);
    history.addHistory(
        [record('a', 1, 'repeat'), record('b', 1, 'repeat')].join('\n'));
    history.addLive(record('c', 3, 'last'));
    history.addHistory([
      record('a', 1, 'repeat'),
      record('b', 1, 'repeat'),
      record('c', 2, 'last')
    ].join('\n'));
    expect(history.entries.map((e) => e.message), ['repeat', 'repeat', 'last']);
    history.addLive(record('d', 4, 'new'));
    expect(history.entries.map((e) => e.message), ['repeat', 'last', 'new']);
  });

  testWidgets(
      'settings logs backfill before open and merge pending live overlap',
      (tester) async {
    final pending = Completer<String>();
    var listens = 0;
    messenger.setMockMethodCallHandler(events, (call) async {
      if (call.method == 'listen') listens++;
      return null;
    });
    messenger.setMockMethodCallHandler(vpn, (call) async {
      if (call.method == 'readNativeLogs') return pending.future;
      if (call.method == 'clearNativeLogs') return 'ok';
      return 'ok';
    });
    await tester.pumpWidget(host());
    await tester.pumpAndSettle();
    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('Logs'));
    await tester.tap(find.text('Logs'));
    await tester.pumpAndSettle();
    expect(listens, 1);
    Future<void> emit(String value) async {
      messenger.handlePlatformMessage(events.name,
          const StandardMethodCodec().encodeSuccessEnvelope(value), (_) {});
      await tester.pump();
      await tester.pump();
    }

    await emit(record('overlap', 2001, 'repeat live'));
    await emit(record('next', 2002, 'repeat live'));
    pending.complete([
      record('old', 1000, 'before opening'),
      record('overlap', 2000, 'repeat live'),
    ].join('\n'));
    await tester.pumpAndSettle();
    expect(find.text('before opening'), findsOneWidget);
    expect(find.text('repeat live'), findsNWidgets(2));
    await tester.tap(find.byTooltip('Clear'));
    await tester.pumpAndSettle();
    expect(find.text('before opening'), findsNothing);
    expect(find.text('repeat live'), findsNothing);
    await emit(record('after', 3000, 'after clear'));
    await tester.pumpAndSettle();
    expect(find.text('after clear'), findsOneWidget,
        reason: tester
            .widgetList<Text>(find.byType(Text))
            .map((t) => t.data)
            .join('|'));
    await tester.pumpWidget(const SizedBox());
    await tester.pumpAndSettle();
  });

  testWidgets('clear rejects history read started before clear',
      (tester) async {
    final pending = Completer<String>();
    messenger.setMockMethodCallHandler(
        vpn,
        (call) async =>
            call.method == 'readNativeLogs' ? pending.future : 'ok');
    await tester.pumpWidget(host());
    await tester.pumpAndSettle();
    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.text('Logs'));
    await tester.tap(find.text('Logs'));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Clear'));
    pending.complete(record('old', 1, 'must not return'));
    await tester.pumpAndSettle();
    expect(find.text('must not return'), findsNothing);
    await tester.pumpWidget(const SizedBox());
    await tester.pumpAndSettle();
  });
}
