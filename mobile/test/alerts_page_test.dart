import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/pages/alerts_page.dart';
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
    final key = '${options.method} ${options.path}';
    final queue = responses[key];
    if (queue == null || queue.isEmpty) {
      return ResponseBody.fromString(
          jsonEncode({'error': 'no route: $key'}), 404,
          headers: {
            Headers.contentTypeHeader: [Headers.jsonContentType],
          });
    }
    return queue.removeAt(0);
  }

  @override
  void close({bool force = false}) {}
}

const _alerts = {
  'data': [
    {'id': '1', 'type': 'error', 'title': '宕机', 'message': 'nginx 挂了',
     'read': false},
    {'id': '2', 'type': 'warning', 'title': '临期', 'message': '域名快到期',
     'read': false},
    {'id': '3', 'type': 'success', 'title': '恢复', 'message': '已恢复',
     'read': true},
    {'id': '4', 'type': 'info', 'title': '通知', 'message': '常规通知',
     'read': true},
  ]
};

CockpitApi _api(MockAdapter a) => CockpitApi(ApiClient.forTest(
    Dio(BaseOptions(baseUrl: 'http://test'))..httpClientAdapter = a));

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: const MaterialApp(home: AlertsPage()),
  ));
}

void main() {
  testWidgets('告警页：四类型图标 + 未读圆点', (tester) async {
    final a = MockAdapter()..on('GET', '/api/alerts', 200, _alerts);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    expect(find.text('告警'), findsOneWidget);
    expect(find.text('宕机'), findsOneWidget);
    expect(find.byIcon(Icons.error_outline), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_outlined), findsOneWidget);
    expect(find.byIcon(Icons.check_circle_outline), findsOneWidget);
    expect(find.byIcon(Icons.info_outline), findsOneWidget);
    // 两条未读 → 两个小圆点
    expect(find.byIcon(Icons.circle), findsNWidgets(2));
  });

  testWidgets('告警页：空态与加载失败占位', (tester) async {
    final a = MockAdapter()..on('GET', '/api/alerts', 200, {'data': <Object?>[]});
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();
    expect(find.text('暂无告警'), findsOneWidget);
  });

  testWidgets('告警页：接口失败占位', (tester) async {
    final a = MockAdapter(); // 404
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();
    expect(find.textContaining('加载失败'), findsOneWidget);
  });

  testWidgets('告警页：全部已读成功 → 重新拉列表', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/alerts', 200, _alerts)
      ..on('PUT', '/api/alerts/read-all', 200, {})
      ..on('GET', '/api/alerts', 200, {'data': <Object?>[]});
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('全部已读'));
    await tester.pumpAndSettle();

    expect(find.text('暂无告警'), findsOneWidget);
  });

  testWidgets('告警页：全部已读失败 → SnackBar 报错', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/alerts', 200, _alerts)
      ..on('PUT', '/api/alerts/read-all', 500, {'error': 'nope'})
      ..on('GET', '/api/alerts', 200, _alerts);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('全部已读'));
    await tester.pumpAndSettle();

    expect(find.textContaining('操作失败'), findsOneWidget);
    expect(find.text('宕机'), findsOneWidget); // 刷新后列表仍在
  });

  testWidgets('告警页：下拉刷新', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/alerts', 200, _alerts)
      ..on('GET', '/api/alerts', 200, {'data': <Object?>[]});
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();
    expect(find.text('宕机'), findsOneWidget);

    await tester.fling(find.text('宕机'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('暂无告警'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
