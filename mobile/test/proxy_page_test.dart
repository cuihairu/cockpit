import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/proxy_page.dart';
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
    {'type': 'nginx-proxy', 'metadata': {'version': 'nginx/1.24.0'}},
  ],
};

const _status = {
  'backend': 'nginx',
  'installed': true,
  'version': 'nginx/1.24.0',
  'confDir': '/etc/nginx/conf.d',
  'siteCount': 2,
  'reloadMode': 'systemctl',
};

const _sites = {
  'sites': [
    {
      'name': 'blog',
      'serverNames': ['blog.example.com', 'www.example.com'],
      'upstream': '127.0.0.1:3000',
      'scheme': 'http',
      'websocket': false,
    },
    {
      'name': 'api',
      'serverNames': ['api.example.com'],
      'upstream': '127.0.0.1:8080',
      'scheme': 'https',
      'websocket': true,
    },
  ],
};

const _siteDetail = {
  'name': 'blog',
  'site': {
    'name': 'blog',
    'serverNames': ['blog.example.com'],
    'upstream': '127.0.0.1:3000',
    'scheme': 'http',
    'websocket': false,
  },
  'content': '# cockpit:meta {"name":"blog"}\n\nserver {\n  listen 80;\n}\n',
};

Future<void> _pump(WidgetTester tester, CockpitApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      apiProvider.overrideWith((ref) async => api),
    ],
    child: MaterialApp(
      home: ProxyPage(agent: Agent.fromJson(_agent)),
    ),
  ));
}

void main() {
  testWidgets('反代页：状态卡与站点列表渲染', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('反代 · web-1'), findsOneWidget);
    // 状态卡
    expect(find.textContaining('后端 Nginx'), findsOneWidget);
    expect(find.textContaining('nginx/1.24.0'), findsOneWidget);
    expect(find.textContaining('目录 /etc/nginx/conf.d'), findsOneWidget);
    expect(find.textContaining('站点 2'), findsOneWidget);
    expect(find.textContaining('生效 systemctl'), findsOneWidget);
    // 站点行
    expect(find.text('blog'), findsOneWidget);
    expect(find.text('blog.example.com'), findsOneWidget);
    expect(find.text('www.example.com'), findsOneWidget);
    expect(find.text('127.0.0.1:3000'), findsOneWidget);
    expect(find.text('HTTP'), findsOneWidget);
    expect(find.text('api'), findsOneWidget);
    expect(find.text('api.example.com'), findsOneWidget);
    expect(find.text('HTTPS'), findsOneWidget);
    expect(find.text('WS'), findsOneWidget);
  });

  testWidgets('反代页：空数据占位', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, {
        ..._status,
        'siteCount': 0,
      })
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200,
          {'sites': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.text('暂无站点'), findsOneWidget);
  });

  testWidgets('反代页：加载失败占位 + 下拉刷新恢复', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 503, {'error': 'agent offline'})
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    expect(find.textContaining('加载失败'), findsOneWidget);

    await tester.fling(
        find.textContaining('加载失败'), const Offset(0, 400), 1000);
    await tester.pumpAndSettle();
    expect(find.text('blog'), findsOneWidget);
    expect(find.textContaining('后端 Nginx'), findsOneWidget);
  });

  testWidgets('反代页：点站点行弹配置预览', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites)
      ..on('GET', '/api/agents/ag-1/proxy/sites/blog', 200, _siteDetail);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('blog'));
    await tester.pumpAndSettle();

    expect(find.text('配置预览：blog'), findsOneWidget);
    expect(find.textContaining('# cockpit:meta'), findsOneWidget);
    expect(find.text('关闭'), findsOneWidget);

    await tester.tap(find.text('关闭'));
    await tester.pumpAndSettle();
    expect(find.text('配置预览：blog'), findsNothing);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
