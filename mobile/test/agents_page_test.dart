import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/pages/agents_page.dart';
import 'package:cockpit_mobile/pages/proxy_page.dart';
import 'package:cockpit_mobile/state/settings.dart';

class MockAdapter implements HttpClientAdapter {
  final responses = <String, List<ResponseBody>>{};
  int posts = 0;

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
    if (options.method == 'POST') posts++;
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

const _agent = {
  'id': 'ag-1',
  'hostname': 'web-1',
  'ip': '10.0.0.1',
  'region': 'cn',
  'zone': 'home',
  'status': 'online',
  'capabilities': [
    {'type': 'docker-api'},
    {'type': 'cron'},
    {'type': 'file'},
    {'type': 'nginx-proxy', 'metadata': {'version': 'nginx/1.24.0'}},
    {
      'type': 'remote-services',
      'metadata': {
        'ssh': {'host': '10.0.0.1', 'port': 22, 'running': true},
      },
    },
  ],
};

const _containers = [
  {'ID': 'c1', 'Name': '/web', 'Image': 'nginx', 'State': 'running',
   'Status': 'Up 2h', 'Created': 1758500000},
  {'ID': 'c2', 'Name': '/db', 'Image': 'postgres', 'State': 'exited',
   'Status': 'Exited', 'Created': 1758400000},
];

CockpitApi _api(MockAdapter a) => CockpitApi(ApiClient.forTest(
    Dio(BaseOptions(baseUrl: 'http://test'))..httpClientAdapter = a));

Future<void> _pump(WidgetTester tester, CockpitApi api,
    {Widget? home}) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: MaterialApp(home: home ?? Scaffold(body: AgentsPage())),
  ));
}

void main() {
  testWidgets('主机列表：反代入口图标 → 打开 ProxyPage', (tester) async {
    // 列表 fixture 自带 nginx-proxy capability → 行尾出现反代图标
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent]);
    await _pump(tester, _api(a));
    await tester.pump();
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.lan).first);
    await tester.pumpAndSettle();

    expect(find.byType(ProxyPage), findsOneWidget);
  });


  testWidgets('主机列表：在线/离线渲染 + 能力捷径按钮', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [
        _agent,
        {
          'id': 'ag-2',
          'hostname': 'off-1',
          'ip': '10.0.0.2',
          'status': 'offline',
          'capabilities': <Map<String, dynamic>>[],
        }
      ]);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    expect(find.text('web-1'), findsOneWidget);
    expect(find.text('10.0.0.1 · cn · home'), findsOneWidget);
    expect(find.text('off-1'), findsOneWidget);
    // 无能力主机无捷径按钮；全能主机三个
    expect(find.byIcon(Icons.view_in_ar), findsOneWidget);
    expect(find.byIcon(Icons.schedule), findsOneWidget);
    expect(find.byIcon(Icons.folder_open), findsOneWidget);
  });

  testWidgets('主机列表：空数据占位', (tester) async {
    final a = MockAdapter()..on('GET', '/api/agents', 200, <Object?>[]);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();
    expect(find.text('暂无已注册主机'), findsOneWidget);
  });

  testWidgets('主机列表：加载失败占位', (tester) async {
    final a = MockAdapter(); // 无路由 → 404
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();
    expect(find.textContaining('加载失败'), findsOneWidget);
  });

  testWidgets('主机行点开能力动作单，五入口齐全', (tester) async {
    final a = MockAdapter()..on('GET', '/api/agents', 200, [_agent]);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('web-1'));
    await tester.pumpAndSettle();

    expect(find.text('选择功能'), findsOneWidget);
    expect(find.text('SSH 终端'), findsOneWidget);
    expect(find.text('10.0.0.1:22'), findsOneWidget);
    expect(find.text('容器'), findsOneWidget);
    expect(find.text('定时任务'), findsOneWidget);
    expect(find.text('文件'), findsOneWidget);
    expect(find.text('反代'), findsOneWidget);
  });

  testWidgets('动作单进反代页：状态卡与站点列表', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, {
        'backend': 'nginx',
        'installed': true,
        'version': 'nginx/1.24.0',
        'confDir': '/etc/nginx/conf.d',
        'siteCount': 1,
        'reloadMode': 'systemctl',
      })
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, {
        'sites': [
          {
            'name': 'blog',
            'serverNames': ['blog.example.com'],
            'upstream': '127.0.0.1:3000',
            'scheme': 'http',
            'websocket': false,
          },
        ],
      });
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('web-1'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('反代'));
    await tester.pumpAndSettle();

    expect(find.text('反代 · web-1'), findsOneWidget);
    expect(find.textContaining('后端 Nginx'), findsOneWidget);
    expect(find.text('blog'), findsOneWidget);
  });

  testWidgets('动作单进容器页：列表 + 停止动作 SnackBar + 刷新', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('GET', '/api/docker/agents/ag-1/containers', 200, _containers)
      ..on('POST', '/api/docker/agents/ag-1/containers/c1/stop', 200, {})
      // 动作后刷新的第二轮
      ..on('GET', '/api/docker/agents/ag-1/containers', 200, _containers);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('web-1'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('容器'));
    await tester.pumpAndSettle();

    expect(find.text('容器 · web-1'), findsOneWidget);
    expect(find.text('/web'), findsOneWidget);
    expect(find.text('/db'), findsOneWidget);
    // running 容器有停止/重启；exited 只有启动
    expect(find.byIcon(Icons.play_circle), findsOneWidget);
    expect(find.byIcon(Icons.stop_circle), findsOneWidget);

    await tester.tap(find.byType(PopupMenuButton<String>).first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('停止').last);
    await tester.pumpAndSettle();

    expect(a.posts, 1);
    expect(find.textContaining('stop 完成'), findsOneWidget);
  });

  testWidgets('容器动作失败 → SnackBar 报错', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('GET', '/api/docker/agents/ag-1/containers', 200, _containers)
      ..on('POST', '/api/docker/agents/ag-1/containers/c2/start', 500,
          {'error': 'nope'})
      ..on('GET', '/api/docker/agents/ag-1/containers', 200, _containers);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    // 直达：trailing 容器捷径
    await tester.tap(find.byIcon(Icons.view_in_ar));
    await tester.pumpAndSettle();

    await tester.tap(find.byType(PopupMenuButton<String>).last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('启动').last);
    await tester.pumpAndSettle();

    expect(find.textContaining('start 失败'), findsOneWidget);
  });

  testWidgets('下拉刷新重新拉列表', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('GET', '/api/agents', 200, <Object?>[]);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();
    expect(find.text('web-1'), findsOneWidget);

    await tester.fling(find.text('web-1'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('暂无已注册主机'), findsOneWidget);
  });

  testWidgets('动作单进 SSH 终端页（ticket 失败也无妨，路由已到）',
      (tester) async {
    final a = MockAdapter()..on('GET', '/api/agents', 200, [_agent]);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('web-1'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('SSH 终端'));
    // ticket 走 mock（未配路由即 404），无 WS 挂起，可安全 settle；
    // 否则 Dio connectTimeout Timer 会挂到测试结束触发 timersPending。
    await tester.pumpAndSettle();

    expect(find.text('终端 · 10.0.0.1:22'), findsOneWidget);
  });

  testWidgets('动作单进定时任务页', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('GET', '/api/agents/ag-1/cron/jobs', 200,
          {'jobs': <Object?>[], 'external': ''});
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('web-1'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('定时任务'));
    await tester.pumpAndSettle();

    expect(find.text('定时任务 · web-1'), findsOneWidget);
    expect(find.text('暂无定时任务'), findsOneWidget);
  });

  testWidgets('动作单进文件页', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('POST', '/api/agents/ag-1/files/list', 200,
          {'dir': '/', 'entries': <Object?>[]});
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.text('web-1'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('文件'));
    await tester.pumpAndSettle();

    expect(find.text('文件 · /'), findsOneWidget);
    expect(find.text('空目录'), findsOneWidget);
  });

  testWidgets('trailing 捷径：定时任务 / 文件直达', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('GET', '/api/agents/ag-1/cron/jobs', 200,
          {'jobs': <Object?>[], 'external': ''})
      ..on('POST', '/api/agents/ag-1/files/list', 200,
          {'dir': '/', 'entries': <Object?>[]});
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.schedule));
    await tester.pumpAndSettle();
    expect(find.text('定时任务 · web-1'), findsOneWidget);

    await tester.pageBack();
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.folder_open));
    await tester.pumpAndSettle();
    expect(find.text('文件 · /'), findsOneWidget);
  });

  testWidgets('容器页：加载失败占位与下拉刷新', (tester) async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [_agent])
      ..on('GET', '/api/docker/agents/ag-1/containers', 404, {'error': 'x'})
      ..on('GET', '/api/docker/agents/ag-1/containers', 200, _containers);
    await _pump(tester, _api(a));
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.view_in_ar));
    await tester.pumpAndSettle();
    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(find.textContaining('加载失败'), const Offset(0, 400),
        1000);
    await tester.pumpAndSettle();
    expect(find.text('/web'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
