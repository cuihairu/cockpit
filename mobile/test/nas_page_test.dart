import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/nas_page.dart';
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
  'hostname': 'nas-1',
  'ip': '10.0.0.1',
  'status': 'online',
  'capabilities': [
    {'type': 'nas'},
  ],
};

const _status = {
  'available': true,
  'source': 'linux',
  'pools': [
    {
      'name': 'tank',
      'kind': 'zfs',
      'state': 'healthy',
      'totalGB': 8000,
      'usedGB': 4096,
      'devices': ['sda', 'sdb'],
      'detail': '',
      'host': '',
    },
    {
      'name': 'md0',
      'kind': 'mdadm',
      'state': 'failed',
      'totalGB': 2000,
      'usedGB': 100,
      'devices': ['sdc1', 'sdd1'],
      'detail': 'U_',
      'host': '',
    },
    {
      'name': 'volume_1',
      'kind': 'dsm',
      'state': 'healthy',
      'totalGB': 4000,
      'usedGB': 2000,
      'devices': <String>[],
      'detail': '',
      'host': 'dsm1',
    },
  ],
  'mounts': [
    {
      'device': '/dev/md0',
      'mountPath': '/mnt/pool',
      'fsType': 'ext4',
      'totalGB': 2000,
      'usedGB': 1848,
      'host': '',
    },
    {
      'device': '/dev/sda1',
      'mountPath': '/boot',
      'fsType': 'ext4',
      'totalGB': 100,
      'usedGB': 30,
      'host': '',
    },
    {
      'device': 'tmpfs',
      'mountPath': '/run',
      'fsType': 'tmpfs',
      'totalGB': 0,
      'usedGB': 0,
      'host': '',
    },
  ],
  'shares': [
    {
      'protocol': 'smb',
      'name': 'media',
      'path': '/mnt/pool/media',
      'comment': '影音',
      'hosts': '10.0.0.0/8',
      'host': '',
    },
  ],
};

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  // 三段快照较高：放大测试画布避免 ListView 懒构建吞掉断言目标
  await tester.binding.setSurfaceSize(const Size(800, 1600));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: MaterialApp(
      home: NasPage(agent: Agent.fromJson(_agent)),
    ),
  ));
}

void main() {
  testWidgets('存储观测页：三段渲染、故障池置顶、host 来源与阈值下不高亮',
      (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/nas/status', 200, _status)
      ..on('GET', '/api/nas/config', 200,
          {'scan_interval_seconds': 1800, 'usage_warn_percent': 95});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('存储观测 · nas-1'), findsOneWidget);
    expect(find.text('数据源：linux'), findsOneWidget);
    expect(find.text('存储池'), findsOneWidget);
    expect(find.text('挂载点容量'), findsOneWidget);
    expect(find.text('网络共享'), findsOneWidget);

    // 排序：failed(md0) 置顶，healthy 按原相对顺序在后
    expect(
        tester.getTopLeft(find.text('md0')).dy <
            tester.getTopLeft(find.text('tank')).dy,
        isTrue);
    // kind 标签映射（zfs→ZFS / dsm→DSM）+ 远端来源设备 host 后缀
    expect(find.text('tank'), findsOneWidget);
    expect(find.text('md0'), findsOneWidget);
    expect(find.text('volume_1 · dsm1'), findsOneWidget);
    expect(find.textContaining('ZFS · 容量 7.8 TB · 已用 4.0 TB'), findsOneWidget);
    expect(find.textContaining('DSM · 容量 3.9 TB'), findsOneWidget);
    // 故障标签红显 + detail 第三行 + 成员盘
    expect(find.text('故障'), findsOneWidget);
    expect(find.text('健康'), findsNWidgets(2));
    expect(find.textContaining('成员盘 sdc1、sdd1'), findsOneWidget);
    expect(find.textContaining('成员盘 —'), findsOneWidget);

    // 挂载：92% < 阈值 95 → 不红；对照 30% 不红；totalGB<=0 → '—'
    final hot = tester.widget<Text>(find.text('92%（1.8 TB）'));
    expect(hot.style?.color, isNull);
    final cool = tester.widget<Text>(find.text('30%（30 GB）'));
    expect(cool.style?.color, isNull);
    expect(find.text('—'), findsOneWidget);
    expect(find.textContaining('/dev/md0 · ext4'), findsOneWidget);
    expect(find.text('/mnt/pool'), findsOneWidget);

    // 共享：协议标签 + 说明 + 允许网段
    expect(find.text('SMB · media'), findsOneWidget);
    expect(find.textContaining('/mnt/pool/media · 说明 影音 · 允许 10.0.0.0/8'),
        findsOneWidget);
  });

  testWidgets('存储观测页：config 拉取失败按默认阈值 80，超限红高亮',
      (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/nas/status', 200, _status);
    // /api/nas/config 未注册 → 404 → 页面兜底 80
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    // 92% >= 80 → 红 #cf1322 + w600；30% 仍不红（观测不被配置失败阻塞）
    final hot = tester.widget<Text>(find.text('92%（1.8 TB）'));
    expect(hot.style?.color, const Color(0xFFCF1322));
    expect(hot.style?.fontWeight, FontWeight.w600);
    expect(tester.widget<Text>(find.text('30%（30 GB）')).style?.color, isNull);
  });

  testWidgets('存储观测页：available=false 空态占位', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/nas/status', 200,
          {'available': false, 'source': ''});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('未发现可观测的存储（mdadm/ZFS/LVM/SMB/NFS 均无数据）'),
        findsOneWidget);
  });

  testWidgets('存储观测页：三段全空占位', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/nas/status', 200, {
        'available': true,
        'source': 'linux',
        'pools': <Object?>[],
        'mounts': <Object?>[],
        'shares': <Object?>[],
      });
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('未发现存储池/挂载点/网络共享'), findsOneWidget);
    expect(find.text('存储池'), findsNothing);
  });

  testWidgets('存储观测页：加载失败占位 + 下拉刷新恢复', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/nas/status', 404, {'error': 'x'})
      ..on('GET', '/api/agents/ag-1/nas/status', 200, _status);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(
        find.textContaining('加载失败'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('md0'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
