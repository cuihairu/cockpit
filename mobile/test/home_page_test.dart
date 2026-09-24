import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/main.dart' as app;
import 'package:cockpit_mobile/pages/dashboard_page.dart';
import 'package:cockpit_mobile/pages/home_page.dart';
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

const _storageChannel =
    MethodChannel('plugins.it_nomads.com/flutter_secure_storage');
const _authChannel = MethodChannel('plugins.flutter.io/local_auth');

void main() {
  tearDown(() {
    TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_storageChannel, null);
    TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_authChannel, null);
  });

  testWidgets('HomePage：五 tab 框架渲染与切换', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, <Object?>[])
      ..on('GET', '/api/alerts', 200, {'data': <Object?>[]})
      ..on('GET', '/api/status', 200, <String, dynamic>{})
      ..on('GET', '/api/resources/domains', 200, {'data': <Object?>[]})
      ..on('GET', '/api/resources/certificates', 200, {'data': <Object?>[]})
      ..on('GET', '/api/backups/configs', 200, {'configs': <Object?>[]})
      ..on('GET', '/api/backups/runs', 200, {'runs': <Object?>[]});
    final api = CockpitApi(ApiClient.forTest(
        Dio(BaseOptions(baseUrl: 'http://test'))..httpClientAdapter = a));

    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: MaterialApp(home: HomePage()), // 非 const：命中构造行
    ));
    await tester.pumpAndSettle();

    expect(find.byType(NavigationBar), findsOneWidget);
    for (final label in ['仪表盘', '主机', '资源', '告警', '设置']) {
      expect(find.text(label), findsOneWidget);
    }
    // 首屏是仪表盘层（卡片细节由 dashboard 专属测试覆盖）
    expect(find.byType(DashboardPage), findsOneWidget);

    // 切到主机 tab：IndexedStack 换层，离线图标所在列表可见
    await tester.tap(find.text('主机'));
    await tester.pumpAndSettle();
    expect(find.text('暂无已注册主机'), findsOneWidget);

    // 切到设置 tab
    await tester.tap(find.text('设置'));
    await tester.pumpAndSettle();
    expect(find.text('Server'), findsOneWidget);

    // 非首页 tab 系统返回：PopScope 拦截，回首页 tab 不退出
    await tester.state<NavigatorState>(find.byType(Navigator).first)
        .maybePop();
    await tester.pumpAndSettle();
    expect(
        tester.widget<NavigationBar>(find.byType(NavigationBar)).selectedIndex,
        0);
  });

  testWidgets('CockpitApp：apiProvider 错误 → 回到引导页', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => throw StateError('boom')),
      ],
      child: const app.CockpitApp(),
    ));
    await tester.pumpAndSettle();
    expect(find.text('连接 Cockpit Server'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
