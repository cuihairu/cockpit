import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/files_page.dart';
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
    {'type': 'file'},
  ],
};

const _rootEntries = {
  'dir': '/',
  'entries': [
    {
      'name': 'etc',
      'size': 4096,
      'mode': '0755',
      'uid': 0,
      'gid': 0,
      'mtime': 1758500000,
      'isDir': true,
      'isSymlink': false,
      'target': '',
    },
    {
      'name': 'hosts',
      'size': 221,
      'mode': '0644',
      'mtime': 1758400000,
      'isDir': false,
      'isSymlink': false,
      'target': '',
    },
    {
      'name': 'localtime',
      'size': 0,
      'mode': '0777',
      'mtime': 1758300000,
      'isDir': false,
      'isSymlink': true,
      'target': '/usr/share/zoneinfo/UTC',
    },
  ],
};

const _etcEntries = {
  'dir': '/etc',
  'entries': [
    {
      'name': 'hostname',
      'size': 12,
      'mode': '0644',
      'mtime': 1758400000,
      'isDir': false,
      'isSymlink': false,
      'target': '',
    },
  ],
};

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: MaterialApp(
      home: FilesPage(agent: Agent.fromJson(_agent)),
    ),
  ));
}

void main() {
  testWidgets('文件页：根目录渲染（目录/文件/链接，目录排前）', (tester) async {
    final adapter = MockAdapter()
      ..on('POST', '/api/agents/ag-1/files/list', 200, _rootEntries);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('文件 · /'), findsOneWidget);
    // 排序：目录 etc 在文件 hosts 之前
    expect(tester.getTopLeft(find.text('etc')).dy <
        tester.getTopLeft(find.text('hosts')).dy, isTrue);
    // 文件行 subtitle：大小 · 权限 · 时间
    expect(find.textContaining('221 B'), findsOneWidget);
    expect(find.textContaining('0644'), findsOneWidget);
  });

  testWidgets('文件页：点目录进入子目录，文件弹详情', (tester) async {
    final adapter = MockAdapter()
      ..on('POST', '/api/agents/ag-1/files/list', 200, _rootEntries)
      ..on('POST', '/api/agents/ag-1/files/list', 200, _etcEntries);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('etc'));
    await tester.pumpAndSettle();

    // 第二轮请求 dir=/etc，标题跟随
    expect(find.text('文件 · etc'), findsOneWidget);
    expect(find.text('hostname'), findsOneWidget);

    // 点文件弹详情（路径为 /etc/hostname）
    await tester.tap(find.text('hostname'));
    await tester.pumpAndSettle();
    expect(find.text('路径：/etc/hostname'), findsOneWidget);
    expect(find.textContaining('类型：文件'), findsOneWidget);
  });

  testWidgets('文件页：symlink 详情显示指向目标', (tester) async {
    final adapter = MockAdapter()
      ..on('POST', '/api/agents/ag-1/files/list', 200, _rootEntries);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('localtime'));
    await tester.pumpAndSettle();
    expect(find.text('类型：链接'), findsOneWidget);
    expect(find.text('指向：/usr/share/zoneinfo/UTC'), findsOneWidget);
  });

  testWidgets('文件页：加载失败与空目录占位', (tester) async {
    final adapter = MockAdapter()
      ..on('POST', '/api/agents/ag-1/files/list', 404, {'error': 'x'})
      ..on('POST', '/api/agents/ag-1/files/list', 200,
          {'dir': '/', 'entries': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(
        find.textContaining('加载失败'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('空目录'), findsOneWidget);
  });

  testWidgets('文件页：进子目录后系统返回键回上级（PopScope 拦截）',
      (tester) async {
    final adapter = MockAdapter()
      ..on('POST', '/api/agents/ag-1/files/list', 200, _rootEntries)
      ..on('POST', '/api/agents/ag-1/files/list', 200, _etcEntries)
      ..on('POST', '/api/agents/ag-1/files/list', 200, _rootEntries);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('etc'));
    await tester.pumpAndSettle();
    expect(find.text('文件 · etc'), findsOneWidget);

    // 根 Navigator maybePop：子目录 canPop=false → 拦截并回上级
    await tester
        .state<NavigatorState>(find.byType(Navigator).first)
        .maybePop();
    await tester.pumpAndSettle();
    expect(find.text('文件 · /'), findsOneWidget);
  });

  testWidgets('文件页：GB 级文件尺寸格式化', (tester) async {
    final adapter = MockAdapter()
      ..on('POST', '/api/agents/ag-1/files/list', 200, {
        'dir': '/',
        'entries': [
          {
            'name': 'huge.iso',
            'size': 5 * 1024 * 1024 * 1024,
            'mode': '0644',
            'mtime': 1758400000,
            'isDir': false,
            'isSymlink': false,
            'target': '',
          },
        ],
      });
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('5 GB'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
