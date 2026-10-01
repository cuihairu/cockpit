import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/smart_page.dart';
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
    {'type': 'hardware-monitor', 'metadata': {'smart': true}},
  ],
};

const _status = {
  'available': true,
  'devices': [
    {
      'name': 'sda',
      'model': 'WDC WD40EFRX',
      'serial': 'WD-WCC4E1234567',
      'sizeBytes': 4000787030016,
      'health': 'passed',
      'temperatureC': 38,
      'powerOnHours': 12345,
    },
    {
      'name': 'sdb',
      'model': 'Samsung SSD 860',
      'health': 'failed',
      'reallocatedSectors': 12,
      'pendingSectors': 8,
      'temperatureC': 41,
    },
    {
      'name': 'nvme0n1',
      'health': 'unknown',
      'error': 'smartctl 未能读取该设备（权限或控制器不受支持）',
    },
    {
      // 与 sda 同为 passed：触发同级扇区数倒序的次级排序键
      'name': 'sdc',
      'model': 'Kingston A400',
      'sizeBytes': 480103981056,
      'health': 'passed',
      'reallocatedSectors': 4,
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
      home: SmartPage(agent: Agent.fromJson(_agent)),
    ),
  ));
}

void main() {
  testWidgets('磁盘健康页：异常盘置顶排序与字段渲染', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/smart/status', 200, _status);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('磁盘健康 · web-1'), findsOneWidget);
    // 排序：failed(sdb) > unknown(nvme0n1) > passed(sdc 有扇区计数) > sda
    expect(
        tester.getTopLeft(find.text('sdb')).dy <
            tester.getTopLeft(find.text('nvme0n1')).dy,
        isTrue);
    expect(
        tester.getTopLeft(find.text('nvme0n1')).dy <
            tester.getTopLeft(find.text('sdc')).dy,
        isTrue);
    expect(
        tester.getTopLeft(find.text('sdc')).dy <
            tester.getTopLeft(find.text('sda')).dy,
        isTrue);
    // 健康标签与扇区计数
    expect(find.text('FAILED'), findsOneWidget);
    expect(find.text('健康'), findsNWidgets(2)); // sda + sdc
    expect(find.text('未知'), findsOneWidget);
    expect(find.textContaining('重映射 12 · 待定扇区 8'), findsOneWidget);
    // 缺省字段落 '—'：sda 与 unknown 盘都无扇区计数
    expect(find.textContaining('重映射 — · 待定扇区 —'), findsNWidgets(2));
    // 容量 1000 进制：4000787030016B ≈ 3726 GB → 3.7 TB；480GB 盘落 GB 档
    expect(find.textContaining('容量 3.7 TB'), findsOneWidget);
    expect(find.textContaining('容量 447 GB'), findsOneWidget);
    expect(find.textContaining('温度 38°C'), findsOneWidget);
    expect(find.textContaining('通电 12345h'), findsOneWidget);
    // unknown 盘附错误说明行
    expect(find.textContaining('smartctl 未能读取'), findsOneWidget);
  });

  testWidgets('磁盘健康页：available=false 空态占位', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/smart/status', 200,
          {'available': false, 'devices': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('未发现可读 SMART 的磁盘（需 root 权限运行 Agent）'),
        findsOneWidget);
  });

  testWidgets('磁盘健康页：加载失败占位 + 下拉刷新恢复', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/smart/status', 404, {'error': 'x'})
      ..on('GET', '/api/agents/ag-1/smart/status', 200, _status);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(
        find.textContaining('加载失败'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('sdb'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
