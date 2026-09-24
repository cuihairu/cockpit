import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/pages/backups_tab.dart';
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

const _config = {
  'id': 1,
  'agent_id': 'ag-1',
  'name': 'etc-backup',
  'sources': ['/etc'],
  'dest_dir': '/var/backups',
  'schedule': 'daily@03:00',
  'retention': 7,
  'enabled': true,
  'last_status': 'success',
  'last_run_at': 1758500000,
  'next_run_at': 1758586400,
  'created_at': 1750000000,
};

const _run = {
  'id': 10,
  'configId': 1,
  'taskId': 't-1',
  'status': 'failed',
  'file': 'etc-backup.tar.gz',
  'size': 1048576,
  'error': 'disk full',
  'remoteStatus': 'failed',
  'startedAt': 1758500000,
  'finishedAt': 1758500600,
};

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: const MaterialApp(home: Scaffold(body: BackupsTab())),
  ));
}

void main() {
  testWidgets('备份 tab：最近运行与任务列表渲染', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/backups/configs', 200, {'configs': [_config]})
      ..on('GET', '/api/backups/runs', 200, {'runs': [_run]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('最近运行'), findsOneWidget);
    expect(find.text('备份任务'), findsOneWidget);
    expect(find.text('etc-backup'), findsOneWidget);
    // 失败运行：错误与异地推送失败都呈现
    expect(find.textContaining('disk full'), findsOneWidget);
    expect(find.textContaining('异地推送失败'), findsOneWidget);
    expect(find.text('失败'), findsOneWidget);
    // 任务行：last_status=success 呈现在 subtitle 拼接文本中
    expect(find.textContaining('成功'), findsOneWidget);
    expect(find.textContaining('下次'), findsOneWidget);
  });

  testWidgets('备份 tab：手动触发 → POST run → 刷新', (tester) async {
    final adapter = MockAdapter()
      // 首轮加载
      ..on('GET', '/api/backups/configs', 200, {'configs': [_config]})
      ..on('GET', '/api/backups/runs', 200, {'runs': <Object?>[]})
      ..on('POST', '/api/backups/configs/1/run', 200, {'status': 'started'})
      // 触发后刷新的第二轮
      ..on('GET', '/api/backups/configs', 200, {'configs': [_config]})
      ..on('GET', '/api/backups/runs', 200, {'runs': [_run]});

    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.play_arrow));
    await tester.pumpAndSettle();

    expect(adapter.posts, 1);
    expect(find.textContaining('已触发（started）'), findsOneWidget);
    // 刷新后「最近运行」段出现（第二轮 runs 有数据）
    expect(find.text('最近运行'), findsOneWidget);
    expect(find.text('失败'), findsOneWidget);
  });

  testWidgets('备份 tab：加载失败占位', (tester) async {
    final adapter = MockAdapter(); // configs/runs 全 404
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);
    expect(find.text('备份任务'), findsNothing);
  });

  testWidgets('备份 tab：无任务与无运行', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/backups/configs', 200, {'configs': <Object?>[]})
      ..on('GET', '/api/backups/runs', 200, {'runs': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('暂无备份任务'), findsOneWidget);
    expect(find.text('最近运行'), findsNothing);
  });

  testWidgets('备份 tab：手动触发失败 → SnackBar 报错', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/backups/configs', 200, {'configs': [_config]})
      ..on('GET', '/api/backups/runs', 200, {'runs': <Object?>[]})
      ..on('POST', '/api/backups/configs/1/run', 500, {'error': 'busy'})
      ..on('GET', '/api/backups/configs', 200, {'configs': <Object?>[]})
      ..on('GET', '/api/backups/runs', 200, {'runs': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.play_arrow));
    await tester.pumpAndSettle();

    expect(find.textContaining('触发失败'), findsOneWidget);
  });

  testWidgets('备份 tab：运行中/超时/未知状态与 GB 尺寸', (tester) async {
    final bigRun = {
      'id': 11,
      'configId': 2,
      'status': 'running',
      'size': 5 * 1024 * 1024 * 1024,
      'error': '',
      'remoteStatus': 'ok',
      'startedAt': 1758500000,
      'finishedAt': 0,
    };
    final timeoutRun = {
      'id': 12,
      'configId': 2,
      'status': 'timeout',
      'size': 512,
      'error': '',
      'remoteStatus': '',
      'startedAt': 1758500100,
      'finishedAt': 1758500200,
    };
    final tbRun = {
      'id': 14,
      'configId': 2,
      'status': 'success',
      'size': 2 * 1024 * 1024 * 1024 * 1024,
      'error': '',
      'remoteStatus': 'ok',
      'startedAt': 1758500500,
      'finishedAt': 1758500600,
    };
    final weirdRun = {
      'id': 13,
      'configId': 2,
      'status': 'weird',
      'size': 0,
      'error': '',
      'remoteStatus': '',
      'startedAt': 1758500300,
      'finishedAt': 1758500400,
    };
    final adapter = MockAdapter()
      ..on('GET', '/api/backups/configs', 200, {
        'configs': [
          _config,
          {
            ..._config,
            'id': 2,
            'name': 'weird-task',
            'schedule': 'manual',
            'next_run_at': 0,
            'last_status': 'weird',
          },
        ]
      })
      ..on('GET', '/api/backups/runs', 200,
          {'runs': [bigRun, timeoutRun, weirdRun, tbRun]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    // 三种状态的 chip 文案
    expect(find.text('运行中'), findsOneWidget);
    expect(find.text('超时'), findsOneWidget);
    // 未知状态：run chip 精确文本 + 任务名 weird-task + 任务 subtitle 各含 weird
    expect(find.text('weird'), findsOneWidget);
    expect(find.textContaining('weird'), findsNWidgets(3));
    // GB 级尺寸与异地已推送
    expect(find.textContaining('5.00 GB'), findsOneWidget);
    expect(find.textContaining('异地已推送'), findsNWidgets(2));
    // TB 级尺寸
    expect(find.textContaining('2.00 TB'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
