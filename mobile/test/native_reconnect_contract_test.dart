import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import '../integration_test/native_reconnect_contract.dart';

// Recording fakes prove flow ordering only, not native service/device behavior.
class RecordingNative {
  final calls = <String>[];
  final initialStart = Completer<String>();
  final initialStop = Completer<String>();
  var starts = 0;
  var stops = 0;
  bool running = false;
  String replacement = 'started';
  String finalStop = 'stopped';
  bool coreReady = true;

  Future<String> start() async {
    calls.add('start');
    if (++starts == 1) return initialStart.future;
    running = replacement == 'started';
    return replacement;
  }

  Future<String> stop() async {
    calls.add('stop');
    if (++stops == 1) return initialStop.future;
    if (finalStop == 'stopped') running = false;
    return finalStop;
  }

  Future<Map<String, dynamic>> info() async {
    calls.add(running ? 'on' : 'off');
    return {
      'vpn_running': running,
      'mihomo_running': running,
      'mihomo_pid': running ? 123 : -1,
    };
  }

  Future<void> ready() async {
    calls.add('ready');
    if (!coreReady) throw StateError('not ready');
  }

  Future<void> run({Duration timeout = const Duration(seconds: 1)}) =>
      assertNativeRapidReconnect(
        start: start,
        stop: stop,
        systemInfo: info,
        assertCoreReady: ready,
        timeout: timeout,
      );
}

Future<void> flush() => Future<void>.delayed(Duration.zero);

void main() {
  for (final outcome in [
    'started',
    'error:VPN startup cancelled',
    'error:VPN startup superseded',
  ]) {
    test('$outcome: pending STOP blocks replacement; both complete', () async {
      final fake = RecordingNative();
      final flow = fake.run();
      await flush();
      expect(fake.calls, ['off', 'start', 'stop']);
      fake.initialStart.complete(outcome);
      await flush();
      expect(fake.starts, 1);
      fake.initialStop.complete('stopped');
      await flow;
      expect(fake.calls, [
        'off',
        'start',
        'stop',
        'off',
        'start',
        'on',
        'ready',
        'stop',
        'off'
      ]);
      expect(fake.running, isFalse);
    });
  }

  test('STOP completing first still waits for initial START', () async {
    final fake = RecordingNative();
    final flow = fake.run();
    await flush();
    fake.initialStop.complete('stopped');
    await flush();
    expect(fake.starts, 1);
    fake.initialStart.complete('error:VPN startup cancelled');
    await flow;
    expect(fake.starts, 2);
  });

  for (final outcome in ['permission_needed', 'error:broken', 'starting']) {
    test('initial $outcome cannot continue', () async {
      final fake = RecordingNative();
      final expectation = expectLater(fake.run(), throwsStateError);
      fake.initialStart.complete(outcome);
      fake.initialStop.complete('stopped');
      await expectation;
      expect(fake.starts, 1);
    });
  }

  test('failed STOP cannot dispatch replacement', () async {
    final fake = RecordingNative();
    final expectation = expectLater(fake.run(), throwsStateError);
    fake.initialStart.complete('started');
    fake.initialStop.complete('error:cleanup failed');
    await expectation;
    expect(fake.starts, 1);
  });

  test('hung STOP is bounded and cannot dispatch replacement', () async {
    final fake = RecordingNative();
    final expectation = expectLater(
        fake.run(timeout: const Duration(milliseconds: 20)),
        throwsA(isA<TimeoutException>()));
    fake.initialStart.complete('started');
    await expectation;
    expect(fake.starts, 1);
  });

  test('hung START is bounded even when STOP completes', () async {
    final fake = RecordingNative();
    final expectation = expectLater(
        fake.run(timeout: const Duration(milliseconds: 20)),
        throwsA(isA<TimeoutException>()));
    fake.initialStop.complete('stopped');
    await expectation;
    expect(fake.starts, 1);
  });

  test('replacement permission_needed is not success', () async {
    final fake = RecordingNative()..replacement = 'permission_needed';
    final expectation = expectLater(fake.run(), throwsStateError);
    fake.initialStart.complete('started');
    fake.initialStop.complete('stopped');
    await expectation;
    expect(fake.calls, isNot(contains('ready')));
  });

  test('readiness failure still stops replacement and fails', () async {
    final fake = RecordingNative()..coreReady = false;
    final expectation = expectLater(fake.run(), throwsStateError);
    fake.initialStart.complete('started');
    fake.initialStop.complete('stopped');
    await expectation;
    expect(fake.stops, 2);
    expect(fake.running, isFalse);
  });

  test('final STOP failure cannot pass', () async {
    final fake = RecordingNative()..finalStop = 'error:cleanup failed';
    final expectation = expectLater(fake.run(), throwsStateError);
    fake.initialStart.complete('started');
    fake.initialStop.complete('stopped');
    await expectation;
    expect(fake.running, isTrue);
  });
}
