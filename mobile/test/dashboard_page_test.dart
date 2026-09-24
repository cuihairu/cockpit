import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/pages/dashboard_page.dart';
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

CockpitApi _api(MockAdapter a) => CockpitApi(ApiClient.forTest(
    Dio(BaseOptions(baseUrl: 'http://test'))..httpClientAdapter = a));

/// 三接口齐全的一轮仪表盘数据。
MockAdapter _fullData() => MockAdapter()
  ..on('GET', '/api/agents', 200, [
    {'id': 'a1', 'hostname': 'on', 'ip': '1.1.1.1', 'status': 'online'},
    {'id': 'a2', 'hostname': 'off', 'ip': '2.2.2.2', 'status': 'offline'},
  ])
  ..on('GET', '/api/alerts', 200, {
    'data': [
      {'id': '1', 'type': 'error', 'title': 'e', 'message': 'm', 'read': false},
      {'id': '2', 'type': 'warning', 'title': 'w', 'message': 'm',
       'read': false},
      {'id': '3', 'type': 'success', 'title': 's', 'message': 'm',
       'read': true},
    ]
  })
  ..on('GET', '/api/status', 200, {
    'infrastructure': {'total': 2, 'online': 1},
    'domains': {'valid': 3, 'expiring': 0},
    'certificates': {'valid': 2, 'expiring': 0},
    'services': {'down': 0},
  });

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: const MaterialApp(home: Scaffold(body: DashboardPage())),
  ));
}

void main() {
  testWidgets('仪表盘：六卡片计数与提示语', (tester) async {
    await _pump(tester, _api(_fullData()));
    await tester.pumpAndSettle();

    expect(find.text('主机在线'), findsOneWidget);
    // 数字 1 出现两处：主机在线 1、错误告警 1
    expect(find.text('1'), findsNWidgets(2));
    expect(find.text('/ 2'), findsOneWidget);
    expect(find.text('未读告警'), findsOneWidget);
    expect(find.text('2'), findsOneWidget);
    expect(find.text('需关注'), findsOneWidget);
    expect(find.text('错误告警'), findsOneWidget);
    expect(find.text('尽快处理'), findsOneWidget);
    expect(find.text('服务宕机'), findsOneWidget);
    expect(find.text('正常'), findsWidgets);
    expect(find.text('域名临期'), findsOneWidget);
    expect(find.text('/ 3 正常'), findsOneWidget);
    expect(find.text('证书临期'), findsOneWidget);
    expect(find.text('/ 2 正常'), findsOneWidget);
  });

  testWidgets('仪表盘：清爽零告警态', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, <Object?>[])
      ..on('GET', '/api/alerts', 200, {'data': <Object?>[]})
      ..on('GET', '/api/status', 200, {
        'services': {'down': 0},
      });
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    expect(find.text('清爽'), findsOneWidget);
    expect(find.text('无'), findsOneWidget);
  });

  testWidgets('仪表盘：apiProvider 未就绪 → loading 占位', (tester) async {
    // override 返回永不完成的 future：任何帧都是 loading 分支
    // （async 闭包会在微任务内完成，捕捉不到 loading 帧）
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) => Completer<CockpitApi>().future),
      ],
      child: const MaterialApp(home: Scaffold(body: DashboardPage())),
    ));
    await tester.pump();
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    // data 分支的卡片文案未出现 → 页面确实停在 apiProvider loading
    expect(find.text('主机在线'), findsNothing);
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('仪表盘：接口失败 → 加载失败占位', (tester) async {
    final a = MockAdapter(); // 全 404
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();
    expect(find.textContaining('加载失败'), findsOneWidget);
  });

  testWidgets('仪表盘：apiProvider 错误 → 重试按钮触发 invalidate', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => throw StateError('boom')),
      ],
      child: const MaterialApp(home: Scaffold(body: DashboardPage())),
    ));
    await tester.pumpAndSettle();

    expect(find.textContaining('boom'), findsOneWidget);
    expect(find.text('重试'), findsOneWidget);

    await tester.tap(find.text('重试'));
    await tester.pumpAndSettle();
    // invalidate 后仍走同一 override 抛错，页面留在重试态
    expect(find.text('重试'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
