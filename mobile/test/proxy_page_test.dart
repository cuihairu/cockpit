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

  testWidgets('反代页：点站点行读取配置失败 → SnackBar', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites)
      ..on('GET', '/api/agents/ag-1/proxy/sites/blog', 500,
          {'error': 'disk on fire'});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('blog'));
    await tester.pumpAndSettle();

    expect(find.textContaining('读取配置失败'), findsOneWidget);
  });

  testWidgets('反代页：编辑拉取详情失败 → SnackBar', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites)
      ..on('GET', '/api/agents/ag-1/proxy/sites/blog', 500,
          {'error': 'gone'});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('编辑').first);
    await tester.pumpAndSettle();

    expect(find.textContaining('读取站点失败'), findsOneWidget);
    // 编辑器未打开
    expect(find.text('编辑站点 blog'), findsNothing);
  });

  testWidgets('反代页：删除确认弹窗点取消 → 不发删除请求', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('删除').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('取消'));
    await tester.pumpAndSettle();

    expect(find.text('删除站点'), findsNothing);
    expect(adapter.deletes, 0);
    expect(find.text('blog'), findsOneWidget);
  });

  testWidgets('反代页：删除接口失败 → SnackBar 删除失败', (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200, _sites)
      ..on('DELETE', '/api/agents/ag-1/proxy/sites/blog', 500,
          {'error': 'locked'});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await _pump(tester, CockpitApi(ApiClient.forTest(dio)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('删除').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('删除').last);
    await tester.pumpAndSettle();

    expect(find.textContaining('删除失败'), findsOneWidget);
    expect(find.text('blog'), findsOneWidget);
  });

  testWidgets('编辑器：应用请求失败且无 error 字段 → 展示异常 toString',
      (tester) async {
    final adapter = MockAdapter()
      ..on('GET', '/api/agents/ag-1/proxy/status', 200, _status)
      ..on('GET', '/api/agents/ag-1/proxy/sites', 200,
          {'sites': <Object?>[]})
      ..on('PUT', '/api/agents/ag-1/proxy/sites/blog', 502,
          <String, dynamic>{});
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

    // 响应体无 error 字段 → _extractError 回退 e.toString()
    expect(find.text('应用失败'), findsOneWidget);
    expect(find.textContaining('DioException'), findsOneWidget);
  });

  testWidgets('编辑器：未填域名直接应用 → 请至少填写一个域名', (tester) async {
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
    // 名称与上游填好（表单校验通过），但 serverNames 留空
    await tester.enterText(find.byType(TextFormField).at(0), 'blog');
    await tester.enterText(
        find.byType(TextFormField).at(2), '127.0.0.1:3000');
    await tester.ensureVisible(find.byType(FilledButton));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(FilledButton));
    await tester.pumpAndSettle();

    expect(find.text('请至少填写一个域名'), findsOneWidget);
    expect(adapter.puts, 0);
  });

  testWidgets('编辑器：切 HTTPS → 证书/私钥必填校验 + WebSocket 开关切换',
      (tester) async {
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

    // WebSocket 开关：默认关，点开后打开
    expect(tester.widget<Switch>(find.byType(Switch)).value, isFalse);
    await tester.tap(find.byType(Switch));
    await tester.pumpAndSettle();
    expect(tester.widget<Switch>(find.byType(Switch)).value, isTrue);

    // 切到 HTTPS：证书/私钥字段出现（segment label 经内部包装，
    // 用 buttonStyle 文本兜底直接按文本找）
    await tester.tap(find.text('HTTPS'));
    await tester.pumpAndSettle();
    expect(find.text('证书路径'), findsOneWidget);
    expect(find.text('私钥路径'), findsOneWidget);

    // 证书/私钥留空提交 → 两个 validator 报错
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

    expect(find.text('https 需要证书绝对路径'), findsOneWidget);
    expect(find.text('https 需要私钥绝对路径'), findsOneWidget);
    expect(adapter.puts, 0);
  });

  testWidgets('编辑器：回车添加域名 + 删除 chip + 非法域名提示', (tester) async {
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

    // 域名输入框回车（onFieldSubmitted）添加 chip
    await tester.enterText(
        find.byType(TextFormField).at(1), 'blog.example.com');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();
    expect(find.byType(InputChip), findsOneWidget);

    // 添加一个非法域名 → 红字格式提示
    await tester.enterText(
        find.byType(TextFormField).at(1), 'bad domain!');
    await tester.tap(find.byTooltip('添加域名'));
    await tester.pumpAndSettle();
    expect(find.text('域名只能含字母/数字/点/连字符/通配符 *'), findsOneWidget);

    // 删除 chip（chip 内删除图标不固定，用 chip 内 Icon 定位）
    // → 全空后回到「请输入至少一个域名」提示
    for (var i = 0; i < 2; i++) {
      await tester.tap(find.descendant(
              of: find.byType(InputChip), matching: find.byType(Icon))
          .first);
      await tester.pumpAndSettle();
    }
    expect(find.byType(InputChip), findsNothing);
    expect(find.text('请输入至少一个域名'), findsOneWidget);
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
