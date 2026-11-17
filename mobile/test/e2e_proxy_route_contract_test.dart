import 'package:flutter_test/flutter_test.dart';
import '../integration_test/proxy_route_contract.dart';

Map<String, dynamic> fixture({String target = 'Tokyo-AnyTLS'}) => {
      'proxies': <String, dynamic>{
        'Tokyo-AnyTLS': <String, dynamic>{'type': 'AnyTLS'},
        '🚀 节点选择': <String, dynamic>{'type': 'Selector', 'now': target},
        '♻️ 自动选择': <String, dynamic>{'type': 'URLTest', 'now': 'Tokyo-AnyTLS'},
        'DIRECT': <String, dynamic>{'type': 'Direct'},
      },
    };

void main() {
  test('single AnyTLS route succeeds without requiring three nodes', () {
    expect(() => assertExpectedAnyTlsRoute(fixture(), 'Tokyo-AnyTLS'),
        returnsNormally);
  });
  test('nested url-test selection reaches the only real node', () {
    expect(
        () => assertExpectedAnyTlsRoute(
            fixture(target: '♻️ 自动选择'), 'Tokyo-AnyTLS'),
        returnsNormally);
  });
  test('DIRECT selection is a hard failure even if the node is registered', () {
    expect(
        () => assertExpectedAnyTlsRoute(
            fixture(target: 'DIRECT'), 'Tokyo-AnyTLS'),
        throwsStateError);
  });
  test('missing selected target fails', () {
    expect(
        () => assertExpectedAnyTlsRoute(
            fixture(target: 'missing'), 'Tokyo-AnyTLS'),
        throwsStateError);
  });
  test('cyclic selection fails rather than hanging', () {
    expect(
        () => assertExpectedAnyTlsRoute(
            fixture(target: '🚀 节点选择'), 'Tokyo-AnyTLS'),
        throwsStateError);
  });
  test('wrong transport cannot pass as the subscription node', () {
    final data = fixture();
    data['proxies']['Tokyo-AnyTLS']['type'] = 'Direct';
    expect(() => assertExpectedAnyTlsRoute(data, 'Tokyo-AnyTLS'),
        throwsStateError);
  });
  test('Tor replacement cannot pass real subscription acceptance', () {
    final data = <String, dynamic>{
      'proxies': <String, dynamic>{
        'ci-tor': <String, dynamic>{'type': 'Socks5'}
      }
    };
    expect(() => assertExpectedAnyTlsRoute(data, 'Tokyo-AnyTLS'),
        throwsStateError);
  });
  test('missing registry fails', () {
    expect(
        () => assertExpectedAnyTlsRoute({}, 'Tokyo-AnyTLS'), throwsStateError);
  });
}
