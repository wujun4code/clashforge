import 'package:clashforge_mobile/update_checker.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  group('UpdateInfo', () {
    test('same beta version is not newer', () {
      const info = UpdateInfo(
        tag: 'v0.1.0-beta.20',
        name: 'ClashForge Mobile v0.1.0-beta.20',
        htmlUrl: 'https://example.test/release',
        body: '',
      );

      expect(info.isNewerThan('0.1.0-beta.20'), isFalse);
    });

    test('newer beta version is newer', () {
      const info = UpdateInfo(
        tag: 'v0.1.0-beta.20',
        name: 'ClashForge Mobile v0.1.0-beta.20',
        htmlUrl: 'https://example.test/release',
        body: '',
      );

      expect(info.isNewerThan('0.1.0-beta.19'), isTrue);
    });
  });

  group('fetchLatestRelease', () {
    test('uses releases list so beta releases can be treated as latest', () async {
      final client = MockClient((request) async {
        expect(request.url.path, endsWith('/releases'));
        return http.Response(
          '[{"tag_name":"v0.1.0-beta.20","name":"beta 20",'
          '"html_url":"https://example.test/beta20","body":"","draft":false}]',
          200,
        );
      });

      final info = await fetchLatestRelease(client: client);

      expect(info, isNotNull);
      expect(info!.tag, 'v0.1.0-beta.20');
      expect(info.isNewerThan('0.1.0-beta.20'), isFalse);
    });

    test('falls back to latest endpoint when releases list is unavailable',
        () async {
      var calls = 0;
      final client = MockClient((request) async {
        calls++;
        if (request.url.path.endsWith('/releases')) {
          return http.Response('not found', 404);
        }
        expect(request.url.path, endsWith('/releases/latest'));
        return http.Response(
          '{"tag_name":"v0.1.0-beta.19","name":"beta 19",'
          '"html_url":"https://example.test/beta19","body":""}',
          200,
        );
      });

      final info = await fetchLatestRelease(client: client);

      expect(calls, 2);
      expect(info, isNotNull);
      expect(info!.tag, 'v0.1.0-beta.19');
    });
  });
}
