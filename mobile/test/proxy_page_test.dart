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
  int puts = 0;
  int deletes = 0;

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
    if (options.method == 'PUT') puts++;
    if (options.method == 'DELETE') deletes++;
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
    expect(find.textContaining('后端 Nginx'), findsOneWidget);
    expect(find.textContaining('nginx/1.24.0'), findsOneWidget);
    expect(find.textContaining('目录 /etc/nginx/conf.d'), findsOneWidget);
    expect(find.textContaining('站点 2'), findsOneWidget);
    expect(find.textContaining('生效 systemctl'), findsOneWidget);
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
      ..on('GET', '/api/agents/ag-1/proxy/status', 200,
          {..._status, 'siteCount': 0})
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
      ..on('GET', '/api/agents/ag-1/proxy/status', 503,
          {'error': 'agent offline'})
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

  testWidgets('反代页：新建站点表单校验失败文案', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200,
          {'sites': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('新建站点'));
    await tester.pumpAndSettle();

    expect(find.text('新建站点'), findsOneWidget);
    expect(find.text('站点名称'), findsOneWidget);

    await tester.ensureVisible(find.byType(FilledButton));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(FilledButton));
    await tester.pumpAndSettle();
    expect(find.text('请输入站点名称'), findsOneWidget);
    expect(find.text('请输入上游地址'), findsOneWidget);

    await tester.enterText(find.byType(TextFormField).first, 'Bad Name');
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byType(FilledButton));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(FilledButton));
    await tester.pumpAndSettle();
    expect(find.text('小写字母/数字开头，可用 - 和 _，最长 64 字符'),
        findsOneWidget);
  });

  testWidgets('反代页：新建站点成功 → SnackBar + 列表刷新', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200,
          {'sites': <Object?>[]})
      ..on('PUT', '/api/agents/ag-1/proxy/sites/blog', 200,
          {'name': 'blog', 'file': 'cockpit-site-blog.conf'})
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
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
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('新建站点'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextFormField).at(0), 'blog');
    await tester.enterText(
        find.byType(TextFormField).at(1), 'blog.example.com');
    await tester.tap(find.byTooltip('添加域名'));
    await tester.pumpAndSettle();
    await tester.enterText(
        find.byType(TextFormField).at(2), '127.0.0.1:3000');

    await tester.ensureVisible(find.byType(FilledButton));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(FilledButton));
    await tester.pumpAndSettle();

    expect(find.text('站点 blog 已应用'), findsOneWidget);
    expect(adapter.puts, 1);
    expect(find.text('blog'), findsOneWidget);
  });

  testWidgets('反代页：编辑预填 + name 禁用', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites)
      ..on('GET', '/api/agents/ag-1/proxy/sites/blog', 200, _siteDetail);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('编辑').first);
    await tester.pumpAndSettle();

    expect(find.text('编辑站点 blog'), findsOneWidget);
    final nameField =
        tester.widget<TextFormField>(find.byType(TextFormField).first);
    expect(nameField.controller!.text, 'blog');
    expect(nameField.enabled, isFalse);
  });

  testWidgets('反代页：删除确认 + 成功', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites)
      ..on('DELETE', '/api/agents/ag-1/proxy/sites/blog', 200, {})
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200,
          {'sites': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('删除').first);
    await tester.pumpAndSettle();

    expect(find.text('删除站点'), findsOneWidget);
    expect(find.text('将从 Nginx 移除 blog，确认？'), findsOneWidget);

    await tester.tap(find.text('删除').last);
    await tester.pumpAndSettle();

    expect(find.text('站点 blog 已删除'), findsOneWidget);
    expect(adapter.deletes, 1);
    expect(find.text('暂无站点'), findsOneWidget);
  });

  testWidgets('反代页：应用失败 → 表单内 Alert 展示错误原文', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200,
          {'sites': <Object?>[]})
      ..on('PUT', '/api/agents/ag-1/proxy/sites/blog', 502, {
        'error': 'nginx -t failed: unknown directive "foo"',
      });
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('新建站点'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextFormField).at(0), 'blog');
    await tester.enterText(
        find.byType(TextFormField).at(1), 'blog.example.com');
    await tester.tap(find.byTooltip('添加域名'));
    await tester.pumpAndSettle();
    await tester.enterText(
        find.byType(TextFormField).at(2), '127.0.0.1:3000');

    await tester.ensureVisible(find.byType(FilledButton));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(FilledButton));
    await tester.pumpAndSettle();

    expect(find.text('应用失败'), findsOneWidget);
    expect(find.textContaining('nginx -t failed: unknown directive'),
        findsOneWidget);
    expect(find.text('创建'), findsOneWidget);
  });

  testWidgets('反代页：Traefik 后端 extra 禁用', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, {
        ..._status,
        'backend': 'traefik',
        'reloadMode': 'hot',
        'confDir': '/etc/traefik/dynamic',
      })
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200,
          {'sites': <Object?>[]});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('新建站点'));
    await tester.pumpAndSettle();

    expect(find.text('Traefik 后端不支持高级指令，留空即可'), findsOneWidget);
    final extraField =
        tester.widget<TextFormField>(find.byType(TextFormField).last);
    expect(extraField.enabled, isFalse);
  });
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
