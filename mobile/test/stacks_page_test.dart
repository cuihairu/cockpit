import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/stacks_page.dart';
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

const _agent = {
  'id': 'ag-1',
  'hostname': 'docker-1',
  'ip': '10.0.0.5',
  'status': 'online',
  'capabilities': [
    {'type': 'docker-api'},
  ],
};

const _stacksResult = {
  'stacks': [
    {
      'agentId': 'ag-1',
      'agentName': 'docker-1',
      'name': 'blog',
      'running': 2,
      'total': 2,
      'lastAction': 'up',
      'lastStatus': 'success',
      'lastDeployedAt': 1758586800,
      'online': true,
    },
    {
      'agentId': 'ag-1',
      'agentName': 'docker-1',
      'name': 'monitor',
      'running': 1,
      'total': 3,
      'lastAction': 'down',
      'lastStatus': 'failed',
      'lastDeployedAt': 0,
      'online': true,
    },
  ],
  'info': {'dir': '/opt/stacks', 'dirWritable': true},
};

const _composeResult = {
  'name': 'blog',
  'compose': 'services:\n  web:\n    image: nginx:latest\n',
  'env': 'KEY=v\n',
  'composeFile': '/opt/stacks/blog/compose.yml',
  'modifiedAt': 1758586800,
};

/// up 成功重载后的列表：多出 newapp 条目（完成刷新的可见证据）。
final _stacksResultAfterDeploy = {
  'stacks': [
    ...(_stacksResult['stacks'] as List<dynamic>),
    {
      'agentId': 'ag-1',
      'agentName': 'docker-1',
      'name': 'newapp',
      'running': 0,
      'total': 1,
      'lastAction': '',
      'lastStatus': '',
      'lastDeployedAt': 0,
      'online': true,
    },
  ],
  'info': _stacksResult['info'],
};

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: MaterialApp(
      home: StacksPage(agent: Agent.fromJson(_agent)),
    ),
  ));
}

/// 打开首个 stack（blog）的动作菜单。
Future<void> _openMenu(WidgetTester tester) async {
  await tester.tap(find.byIcon(Icons.more_vert).first);
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('应用部署页：列表渲染 + 自检正常无告警横幅', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('应用部署 · docker-1'), findsOneWidget);
    expect(find.text('blog'), findsOneWidget);
    expect(find.text('monitor'), findsOneWidget);
    expect(find.textContaining('2/2 服务运行'), findsOneWidget);
    expect(find.textContaining('1/3 服务运行'), findsOneWidget);
    expect(find.textContaining('上次 up: success'), findsOneWidget);
    expect(find.textContaining('上次 down: failed'), findsOneWidget);
    expect(find.textContaining('部署时间'), findsNothing);
    expect(find.textContaining('目录异常'), findsNothing);
  });

  testWidgets('应用部署页：stacks 目录自检异常横幅', (tester) async {
    final adapter = MockAdapter()
      ..on(
          'GET',
          '/api/stacks/agents/ag-1',
          200,
          {
            'stacks': <Object?>[],
            'info': {'dir': '/opt/stacks', 'dirWritable': false, 'dirError': 'permission denied'},
          });
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('目录异常'), findsOneWidget);
    expect(find.textContaining('permission denied'), findsOneWidget);
    expect(find.text('/opt/stacks'), findsOneWidget);
    expect(find.text('暂无应用部署'), findsOneWidget);
  });

  testWidgets('应用部署页：加载失败占位 + 下拉刷新恢复', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 404, {'error': 'x'})
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(
        find.textContaining('加载失败'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('blog'), findsOneWidget);
  });

  testWidgets('应用部署页：up 动作 → taskId 轮询 running → success → 完成并刷新', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult) // 首次加载
      ..on('POST', '/api/stacks/agents/ag-1/blog/up', 200,
          {'taskId': 't-1', 'status': 'started'})
      ..on('GET', '/api/stacks/agents/ag-1/tasks/t-1', 200,
          {'id': 't-1', 'stack': 'blog', 'action': 'up', 'status': 'running', 'log': '', 'startedAt': 1, 'finishedAt': 0})
      ..on('GET', '/api/stacks/agents/ag-1/tasks/t-1', 200,
          {'id': 't-1', 'stack': 'blog', 'action': 'up', 'status': 'success', 'log': 'done', 'startedAt': 1, 'finishedAt': 2})
      // 完成后刷新：多出 newapp 证明列表重载真的发生
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResultAfterDeploy);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();
    expect(find.text('newapp'), findsNothing);

    await _openMenu(tester);
    await tester.tap(find.text('启动'));
    // 菜单关闭 + POST 完成 → 提交提示 + 轮询定时器就位（2s 后才到期，settle 不会触发）
    await tester.pumpAndSettle();
    expect(find.textContaining('blog up 已提交'), findsOneWidget);

    await tester.pump(const Duration(seconds: 2)); // 第 1 次轮询 → running
    await tester.pump(const Duration(seconds: 2)); // 第 2 次轮询 → success + 重载
    await tester.pumpAndSettle(); // 排空重载请求与 snackbar 队列
    expect(find.text('newapp'), findsOneWidget);
  });

  testWidgets('应用部署页：轮询单次失败容错，下个周期拿到终态', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult)
      ..on('POST', '/api/stacks/agents/ag-1/blog/up', 200,
          {'taskId': 't-2', 'status': 'started'})
      ..on('GET', '/api/stacks/agents/ag-1/tasks/t-2', 500, {'error': 'boom'})
      ..on('GET', '/api/stacks/agents/ag-1/tasks/t-2', 200,
          {'id': 't-2', 'stack': 'blog', 'action': 'up', 'status': 'success', 'log': '', 'startedAt': 1, 'finishedAt': 2})
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResultAfterDeploy);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await _openMenu(tester);
    await tester.tap(find.text('启动'));
    await tester.pumpAndSettle();

    await tester.pump(const Duration(seconds: 2)); // 500 → 容错跳过
    await tester.pump(const Duration(seconds: 2)); // 重试 → success + 重载
    await tester.pumpAndSettle();
    expect(find.text('newapp'), findsOneWidget);
  });

  testWidgets('应用部署页：动作下发失败 → SnackBar 报错且不进入轮询', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult)
      ..on('POST', '/api/stacks/agents/ag-1/blog/up', 500,
          {'error': 'agent busy'});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await _openMenu(tester);
    await tester.tap(find.text('启动'));
    await tester.pumpAndSettle();

    expect(find.textContaining('blog up 失败'), findsOneWidget);
    // 行内 spinner 复位（_acting 清空），可再次打开菜单
    expect(find.byType(CircularProgressIndicator), findsNothing);
  });

  testWidgets('应用部署页：down 需二次确认，取消不发请求；确认后失败弹日志', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult)
      // 取消不应消费它；确认后才会消费——若取消误发，这里会被吃掉、后面 404
      ..on('POST', '/api/stacks/agents/ag-1/blog/down', 200,
          {'taskId': 't-3', 'status': 'started'})
      ..on('GET', '/api/stacks/agents/ag-1/tasks/t-3', 200,
          {'id': 't-3', 'stack': 'blog', 'action': 'down', 'status': 'failed', 'log': 'compose: error dependency failed to start', 'startedAt': 1, 'finishedAt': 2});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    // 取消：对话框出现且不触发 POST
    await _openMenu(tester);
    await tester.tap(find.text('停止'));
    await tester.pumpAndSettle();
    expect(find.text('停止 blog？'), findsOneWidget);
    expect(find.textContaining('全部 2 个服务'), findsOneWidget);
    await tester.tap(find.text('取消'));
    await tester.pumpAndSettle();
    expect(find.text('停止 blog？'), findsNothing);

    // 确认：POST down → 任务失败 → 日志 bottom sheet
    await _openMenu(tester);
    await tester.tap(find.text('停止'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('停止')); // 对话框确认按钮
    await tester.pumpAndSettle(); // POST 完成 → 提交提示 + 定时器就位
    expect(find.textContaining('blog down 已提交'), findsOneWidget);

    await tester.pump(const Duration(seconds: 2)); // 轮询 → failed → 日志弹层
    await tester.pumpAndSettle();
    expect(find.text('blog down 失败'), findsOneWidget);
    expect(find.textContaining('dependency failed to start'), findsOneWidget);
  });

  testWidgets('应用部署页：compose/.env 只读预览', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult)
      ..on('GET', '/api/stacks/agents/ag-1/blog/compose', 200, _composeResult);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await _openMenu(tester);
    await tester.tap(find.text('查看 compose'));
    await tester.pumpAndSettle();

    expect(find.text('compose · blog'), findsOneWidget);
    expect(find.textContaining('image: nginx:latest'), findsOneWidget);
    expect(find.text('.env'), findsOneWidget);
    expect(find.textContaining('KEY=v'), findsOneWidget);
  });

  testWidgets('应用部署页：compose 加载失败提示', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/stacks/agents/ag-1', 200, _stacksResult)
      ..on('GET', '/api/stacks/agents/ag-1/blog/compose', 502,
          {'error': 'agent unreachable'});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await _openMenu(tester);
    await tester.tap(find.text('查看 compose'));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
