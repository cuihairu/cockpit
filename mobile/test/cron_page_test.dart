import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/cron_page.dart';
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
  'hostname': 'web-1',
  'ip': '10.0.0.1',
  'status': 'online',
  'capabilities': [
    {'type': 'cron'},
  ],
};

const _jobsResult = {
  'jobs': [
    {
      'name': 'logrotate-nightly',
      'schedule': '0 3 * * *',
      'command': '/usr/sbin/logrotate /etc/logrotate.conf',
      'enabled': true,
      'next_run': 1758586800,
    },
    {
      'name': 'old-task',
      'schedule': '*/5 * * * *',
      'command': 'echo hi',
      'enabled': false,
    },
  ],
  'external': '# m h dom mon dow command\n0 0 * * * some-legacy-job\n',
};

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: MaterialApp(
      home: CronJobsPage(agent: Agent.fromJson(_agent)),
    ),
  ));
}

void main() {
  testWidgets('定时任务页：任务列表与外部条目只读渲染', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/cron/jobs', 200, _jobsResult);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('定时任务 · web-1'), findsOneWidget);
    expect(find.text('logrotate-nightly'), findsOneWidget);
    // schedule 与 command 在 subtitle 拼接文本中
    expect(find.textContaining('0 3 * * *'), findsOneWidget);
    expect(find.textContaining('/usr/sbin/logrotate'), findsOneWidget);
    expect(find.textContaining('下次 '), findsOneWidget);
    // 外部条目：段标题 + 原文原样展示
    expect(find.text('外部条目（只读）'), findsOneWidget);
    expect(find.textContaining('some-legacy-job'), findsOneWidget);
    // 禁用任务也在列表中（title 独立可查）
    expect(find.text('old-task'), findsOneWidget);
  });

  testWidgets('定时任务页：空数据占位', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/cron/jobs', 200,
          {'jobs': <Object?>[], 'external': ''});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('暂无定时任务'), findsOneWidget);
    expect(find.text('外部条目（只读）'), findsNothing);
  });

  testWidgets('定时任务页：加载失败占位 + 下拉刷新恢复', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/cron/jobs', 404, {'error': 'x'})
      ..on('GET', '/api/agents/ag-1/cron/jobs', 200, _jobsResult);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(
        find.textContaining('加载失败'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('logrotate-nightly'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
