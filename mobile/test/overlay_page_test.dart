import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/overlay_page.dart';
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
  'hostname': 'gw-1',
  'ip': '10.0.0.1',
  'status': 'online',
  'capabilities': [
    {
      'type': 'overlay',
      'metadata': {
        'identity': {
          'zerotier': {
            'nodeId': 'abc123def',
            'networks': ['net-1', 'net-2'],
          },
          'tailscale': {'id': 'ts-77', 'hostName': 'gw.example.com'},
        },
      },
    },
  ],
};

const _status = {
  'tools': [
    {
      'tool': 'zerotier',
      'status': 'ok',
      'version': '1.14.2',
      'networks': [
        {
          'id': '8056c2e21c',
          'name': 'home',
          'status': 'OK',
          'type': 'PRIVATE',
          'dev': 'zt0',
          'ips': ['10.147.20.10'],
        },
      ],
      'peers': [
        {
          'id': 'a1b2c3',
          'virtualIps': ['10.147.20.15'],
          'latencyMs': 12,
          'online': true,
          'endpoint': '1.2.3.4/9993',
          'role': 'LEAF',
        },
        {
          'id': 'd4e5f6',
          'name': 'office-nas',
          'online': false,
          'endpoint': '5.6.7.8/9993',
        },
      ],
    },
    {
      'tool': 'wireguard',
      'status': 'ok',
      'interfaces': [
        {
          'name': 'wg0',
          'listenPort': '51820',
          'peerCount': 2,
          'peers': [
            {'id': 'pubkey==', 'online': true, 'virtualIps': ['10.9.0.2']},
          ],
        },
      ],
    },
    {
      'tool': 'frp',
      'status': 'degraded',
      'error': 'frps: bind 7000 已被占用',
      'extra': {
        'frpc': {'running': true},
        'frps': {'running': false},
      },
    },
    {'tool': 'tailscale', 'status': 'unavailable'},
  ],
};

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  // 工具卡较多：放大测试画布避免 ListView 懒构建吞掉断言目标
  await tester.binding.setSurfaceSize(const Size(800, 1200));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: MaterialApp(
      home: OverlayPage(agent: Agent.fromJson(_agent)),
    ),
  ));
}

void main() {
  testWidgets('组网观测页：身份芯片、工具卡与 unavailable 过滤', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/overlay/status', 200, _status);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('组网观测 · gw-1'), findsOneWidget);

    // capability metadata 身份芯片
    expect(find.text('ZeroTier abc123def（2 个网络）'), findsOneWidget);
    expect(find.text('Tailscale gw.example.com'), findsOneWidget);

    // 工具卡：标签 + 版本 + 状态
    expect(find.text('ZeroTier · v1.14.2'), findsOneWidget);
    expect(find.text('WireGuard'), findsOneWidget);
    expect(find.text('frp'), findsOneWidget);
    expect(find.text('正常'), findsNWidgets(2));
    expect(find.text('降级'), findsOneWidget);
    // unavailable 的 tailscale 被过滤（身份芯片不受影响）
    expect(find.text('Tailscale'), findsNothing);

    // ZeroTier 网络 + 对端（无名对端落 id；离线对端落 endpoint）
    expect(find.text('home（8056c2e21c）'), findsOneWidget);
    expect(find.textContaining('OK · 10.147.20.10'), findsOneWidget);
    expect(find.text('a1b2c3'), findsOneWidget);
    expect(find.textContaining('10.147.20.15 · 12ms'), findsOneWidget);
    expect(find.text('office-nas'), findsOneWidget);
    expect(find.textContaining('5.6.7.8/9993'), findsOneWidget);

    // WireGuard 接口：peer 计数 + 监听端口（字符串列）
    expect(find.text('wg0'), findsOneWidget);
    expect(find.text('2 个 peer · 监听 51820'), findsOneWidget);

    // frp 进程态 + error 行
    expect(find.text('frpc 运行中 · frps 未运行'), findsOneWidget);
    expect(find.textContaining('frps: bind 7000'), findsOneWidget);
  });

  testWidgets('组网观测页：全 unavailable 空态（身份芯片保留）', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/overlay/status', 200, {
        'tools': [
          {'tool': 'zerotier', 'status': 'unavailable'},
          {'tool': 'tailscale', 'status': 'unavailable'},
        ],
      });
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('未检测到组网工具'), findsOneWidget);
    expect(find.text('ZeroTier abc123def（2 个网络）'), findsOneWidget);
  });

  testWidgets('组网观测页：加载失败占位 + 下拉刷新恢复', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/overlay/status', 404, {'error': 'x'})
      ..on('GET', '/api/agents/ag-1/overlay/status', 200, _status);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(
        find.textContaining('加载失败'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('ZeroTier · v1.14.2'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
