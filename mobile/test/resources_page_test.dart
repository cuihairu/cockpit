import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/pages/resources_page.dart';
import 'package:cockpit_mobile/state/settings.dart';

class MockAdapter implements HttpClientAdapter {
  final responses = <String, List<ResponseBody>>{};

  void on(String method, String path, int status, Object? body) {
    responses.putIfAbsent('$method $path', () => []).add(
          ResponseBody.fromString(jsonEncode(body), status, headers: {
            Headers.contentTypeHeader: [Headers.jsonContentType],
          }),
        );
  }

  @override
  Future<ResponseBody> fetch(
      RequestOptions options,
      Stream<Uint8List>? requestStream,
      Future<void>? cancelFuture) async {
    final queue = responses['${options.method} ${options.path}'];
    if (queue == null || queue.isEmpty) {
      return ResponseBody.fromString(
          jsonEncode({'error': 'no route'}), 404,
          headers: {
            Headers.contentTypeHeader: [Headers.jsonContentType],
          });
    }
    return queue.removeAt(0);
  }

  @override
  void close({bool force = false}) {}
}

ProviderScope _wrap(CockpitApi api) => ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const MaterialApp(home: ResourcesPage()),
    );

void main() {
  testWidgets('资源页：域名 tab 渲染状态徽标与到期日', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/resources/domains', 200, {
        'data': [
          {
            'id': 'dm-1',
            'name': 'example.com',
            'displayName': 'example.com',
            'registrar': 'Cloudflare',
            'registeredDate': '2020-01-01T00:00:00Z',
            'expiryDate': '2027-01-01T00:00:00Z',
            'autoRenew': true,
            'registrarConsoleUrl': '',
            'dnsProvider': 'cloudflare',
            'dnsConsoleUrl': '',
            'certificates': [],
            'subdomains': [],
            'status': 'valid',
          },
          {
            'id': 'dm-2',
            'name': 'legacy.net',
            'displayName': 'legacy.net',
            'registrar': 'GoDaddy',
            'expiryDate': '2026-09-30T00:00:00Z',
            'autoRenew': false,
            'dnsProvider': 'dnspod',
            'status': 'expiring',
          },
        ],
        'total': 2,
        'page': 1,
        'pageSize': 20,
        'totalPages': 1,
      });

    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(_wrap(CockpitApi(ApiClient.forTest(dio))));
    await tester.pumpAndSettle();

    expect(find.text('example.com'), findsOneWidget);
    expect(find.text('legacy.net'), findsOneWidget);
    expect(find.textContaining('到期 2027-01-01'), findsOneWidget);
    expect(find.textContaining('自动续费'), findsOneWidget);
    // 状态徽标：正常 1 个、临期 1 个
    expect(find.text('正常'), findsOneWidget);
    expect(find.text('临期'), findsOneWidget);
  });

  testWidgets('资源页：切到证书 tab 渲染 daysRemaining', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/resources/domains', 200, {
        'data': [],
        'total': 0,
        'page': 1,
        'pageSize': 20,
        'totalPages': 1,
      })
      ..on('GET', '/api/resources/certificates', 200, {
        'data': [
          {
            'id': 'ct-1',
            'name': 'example.com',
            'displayName': 'example.com',
            'type': 'letsencrypt',
            'commonName': 'example.com',
            'sans': ['example.com'],
            'issuedDate': '2026-06-01T00:00:00Z',
            'expiryDate': '2026-10-15T00:00:00Z',
            'autoRenew': true,
            'acmeProvider': 'cloudflare',
            'daysRemaining': 23,
            'status': 'expiring',
          }
        ],
        'total': 1,
        'page': 1,
        'pageSize': 20,
        'totalPages': 1,
      });

    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(_wrap(CockpitApi(ApiClient.forTest(dio))));
    await tester.pumpAndSettle();

    await tester.tap(find.text('证书'));
    await tester.pumpAndSettle();

    expect(find.text('example.com'), findsOneWidget);
    expect(find.textContaining('cloudflare · letsencrypt'), findsOneWidget);
    expect(find.textContaining('到期 2026-10-15（剩 23 天）'), findsOneWidget);
    expect(find.text('临期'), findsOneWidget);
  });

  testWidgets('资源页：已过期徽标 + 下拉刷新（invalidate 后重拉）', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/resources/domains', 200, {
        'data': [
          {
            'id': 'dm-9',
            'name': 'dead.io',
            'registrar': '',
            'expiryDate': '2020-01-01T00:00:00Z',
            'autoRenew': false,
            'dnsProvider': '',
            'status': 'expired',
          }
        ],
        'total': 1,
      })
      // 刷新后（invalidate 重建 apiProvider）的第二轮
      ..on('GET', '/api/resources/domains', 200, {
        'data': <Object?>[],
        'total': 0,
      })
      ..on('GET', '/api/resources/certificates', 200, {
        'data': <Object?>[],
        'total': 0,
      });

    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(_wrap(CockpitApi(ApiClient.forTest(dio))));
    await tester.pumpAndSettle();

    expect(find.text('已过期'), findsOneWidget);

    await tester.fling(find.text('dead.io'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('暂无数据'), findsOneWidget);
  });

  testWidgets('资源页：接口失败占位', (tester) async {
    final adapter = MockAdapter(); // 404
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(_wrap(CockpitApi(ApiClient.forTest(dio))));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
