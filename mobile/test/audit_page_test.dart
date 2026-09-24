import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/audit_page.dart';
import 'package:cockpit_mobile/state/settings.dart';

/// 复用 client_test 的思路：序列响应 mock，页面走真实解析路径。
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

AuditLog _log(int id) => AuditLog(
      id: id,
      userId: 'u-1',
      username: 'admin',
      action: 'action-$id',
      resource: 'agent',
      resourceId: '$id',
      details: '',
      ip: '192.168.1.5',
      status: 'success',
      createdAt: '2026-09-22T03:00:00Z',
    );

Map<String, dynamic> _page(int total, List<AuditLog> logs) => {
      'data': logs
          .map((l) => {
                'id': l.id,
                'user_id': l.userId,
                'username': l.username,
                'action': l.action,
                'resource': l.resource,
                'resource_id': l.resourceId,
                'details': l.details,
                'ip': l.ip,
                'status': l.status,
                'created_at': l.createdAt,
              })
          .toList(),
      'pagination': {'total': total},
    };

void main() {
  testWidgets('审计页：下拉刷新重拉第一页', (tester) async {
    // 首屏 [1,2] → hasMore 预取追加 [3] → 下拉刷新 reset 换成 [3] 单条
    final adapter = MockAdapter()
      ..on('GET', '/api/admin/audit/logs', 200,
          _page(3, [_log(1), _log(2)]))
      ..on('GET', '/api/admin/audit/logs', 200, _page(3, [_log(3)]))
      // reset 拉回的第一页恰好取满 total=2，避免 hasMore 预取无限循环
      ..on('GET', '/api/admin/audit/logs', 200, _page(2, [_log(2), _log(3)]));
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    final api = CockpitApi(ApiClient.forTest(dio));

    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => api),
      ],
      // AlwaysScrollable：数据不足一屏时 RefreshIndicator 也能下拉
      child: MaterialApp(
        scrollBehavior: const MaterialScrollBehavior()
            .copyWith(physics: const AlwaysScrollableScrollPhysics()),
        home: const AuditPage(),
      ),
    ));
    // 收敛首屏 + hasMore 预取，列表为 [1,2,3]
    await tester.pumpAndSettle();
    await tester.pumpAndSettle();
    expect(find.textContaining('admin · action-1'), findsOneWidget);

    await tester.fling(
        find.byType(Scrollable).first, const Offset(0, 400), 1200);
    await tester.pumpAndSettle();

    // reset 后重拉第一页：旧列表被整体替换
    expect(find.textContaining('admin · action-1'), findsNothing);
    expect(find.textContaining('admin · action-3'), findsOneWidget);
  });


  testWidgets('审计页：首屏渲染 + 滚动加载更多', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/admin/audit/logs', 200,
          _page(3, [_log(1), _log(2)]))
      ..on('GET', '/api/admin/audit/logs', 200, _page(3, [_log(3)]));

    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    final api = CockpitApi(ApiClient.forTest(dio));

    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const MaterialApp(home: AuditPage()),
    ));
    await tester.pumpAndSettle();

    // 首屏数据不足一屏（2 条 < total 3），占位 item 进入 cacheExtent，
    // post-frame 自动预取下一页——pumpAndSettle 后应取满 total。
    await tester.pumpAndSettle();

    expect(find.text('admin · action-1'), findsOneWidget);
    expect(find.text('admin · action-2'), findsOneWidget);
    expect(find.text('admin · action-3'), findsOneWidget);
    // total=3 已取完，不再追加 loading 占位
    expect(find.byType(CircularProgressIndicator), findsNothing);
  });

  testWidgets('审计页：失败行红标呈现', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/admin/audit/logs', 200, {
        'data': [
          {
            'id': 9,
            'user_id': 'u-1',
            'username': 'admin',
            'action': 'login',
            'resource': 'session',
            'resource_id': '',
            'details': '',
            'ip': '192.168.1.5',
            'status': 'denied',
            'created_at': '2026-09-22T03:00:00Z',
          }
        ],
        'pagination': {'total': 1},
      });

    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    final api = CockpitApi(ApiClient.forTest(dio));

    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const MaterialApp(home: AuditPage()),
    ));
    await tester.pumpAndSettle();

    expect(find.byIcon(Icons.error_outline), findsOneWidget);
    expect(find.text('admin · login'), findsOneWidget);
  });

  testWidgets('审计页：加载失败占位', (tester) async {
    final adapter = MockAdapter(); // 404
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    final api = CockpitApi(ApiClient.forTest(dio));

    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const MaterialApp(home: AuditPage()),
    ));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);
  });

  testWidgets('审计页：空记录占位', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/admin/audit/logs', 200,
          _page(0, <AuditLog>[]));
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    final api = CockpitApi(ApiClient.forTest(dio));

    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const MaterialApp(home: AuditPage()),
    ));
    await tester.pumpAndSettle();

    expect(find.text('暂无审计记录'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
