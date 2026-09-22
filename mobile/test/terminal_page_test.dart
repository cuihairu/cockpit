import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/models/models.dart';
import 'package:cockpit_mobile/pages/terminal_page.dart';
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

Agent _agentWithSsh() => Agent.fromJson({
      'id': 'ag-1',
      'hostname': 'web-1',
      'ip': '10.0.0.1',
      'status': 'online',
      'capabilities': [
        {
          'type': 'remote-services',
          'metadata': {
            'ssh': {'host': '10.0.0.1', 'port': 22, 'running': true},
          },
        },
      ],
    });

class _UrlSettings extends SettingsNotifier {
  _UrlSettings(this.url);

  final String url;

  @override
  SettingsState build() =>
      SettingsState(serverUrl: url, loaded: true);
}

void main() {
  test('sshService：running 的 ssh 上报解析出 host:port', () {
    final ssh = _agentWithSsh().sshService;
    expect(ssh, isNotNull);
    expect(ssh!.host, '10.0.0.1');
    expect(ssh.port, 22);
  });

  test('sshService：未上报或未运行时为 null', () {
    final noCap = Agent.fromJson({
      'id': 'a',
      'hostname': 'h',
      'ip': '1.1.1.1',
      'status': 'online',
      'capabilities': <Map<String, dynamic>>[
        {
          'type': 'remote-services',
          'metadata': {
            'ssh': {'port': 22, 'running': false},
          },
        },
      ],
    });
    expect(noCap.sshService, isNull);
    final noMeta = Agent.fromJson({
      'id': 'a',
      'hostname': 'h',
      'ip': '1.1.1.1',
      'status': 'online',
      'capabilities': <Map<String, dynamic>>[
        {'type': 'cron'},
      ],
    });
    expect(noMeta.sshService, isNull);
  });

  testWidgets('终端页：ticket 失败显示错误横幅（不连 WS）', (tester) async {
    final adapter = MockAdapter(); // 不注册 /tickets → 404
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _UrlSettings('http://test')),
        apiProvider.overrideWith((ref) async =>
            CockpitApi(ApiClient.forTest(dio))),
      ],
      child: MaterialApp(
        home: TerminalPage(
          agent: _agentWithSsh(),
          ssh: SshService(host: '10.0.0.1', port: 22),
        ),
      ),
    ));
    // connecting 有无限进度条动画；错误态被移除后 pumpAndSettle 才能停
    await tester.pump();
    await tester.pump();
    await tester.pumpAndSettle();

    // 404 → DioException 文本进入错误横幅；重连按钮可点
    expect(find.byIcon(Icons.error_outline), findsOneWidget);
    expect(find.byIcon(Icons.refresh), findsOneWidget);
  });

  testWidgets('终端页：本地 WS 全链路——连通、下行消息、重连、关闭',
      (tester) async {
    // 真 WebSocket 服务端：bind/listen 必须在真 async 区（fake zone 里挂起），
    // flutter_test 的 HttpOverrides 也须先还原，否则真握手被劫持。
    final sessions = <WebSocket>[];
    final inputs = <String>[];
    final upgrades = <String?>[];
    final server = (await tester.runAsync(() async {
      final saved = HttpOverrides.current;
      HttpOverrides.global = null;
      try {
        final s = await HttpServer.bind('127.0.0.1', 0);
        s.listen((req) async {
          final ws = await WebSocketTransformer.upgrade(req,
              protocolSelector: (protocols) => protocols.first);
          upgrades.add(ws.protocol);
          sessions.add(ws);
          ws.add(jsonEncode({'type': 'data', 'data': 'welcome\r\n'}));
          ws.add(jsonEncode({'type': 'error', 'message': 'boom'}));
          ws.add('not-json');
          ws.add(jsonEncode(<Object?>[])); // JSON 但非 Map
          ws.listen((msg) => inputs.add(msg as String));
        });
        return s;
      } finally {
        HttpOverrides.global = saved as HttpOverrides?;
      }
    }))!;

    final adapter = MockAdapter()
      ..on('POST', '/api/remote/tickets', 200,
          {'ticket': 'tk-1', 'expires_at': '2030-01-01T00:00:00Z'})
      ..on('POST', '/api/remote/tickets', 200,
          {'ticket': 'tk-2', 'expires_at': '2030-01-01T00:00:00Z'});
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(
            () => _UrlSettings('http://127.0.0.1:${server.port}')),
        apiProvider.overrideWith(
            (ref) async => CockpitApi(ApiClient.forTest(dio))),
      ],
      child: MaterialApp(
        home: TerminalPage(
          agent: _agentWithSsh(),
          ssh: SshService(host: '10.0.0.1', port: 22),
        ),
      ),
    ));
    // 首轮连接：tickets 走 mock 即完成，IOWebSocketChannel 握手惰性，
    // 页面进 connected（refresh 可见）；真握手只在真 async 区发生——
    // 通过点重连在 runAsync 里触发第二轮，全链路落在真 zone。
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));
    expect(find.byType(LinearProgressIndicator), findsNothing);

    await tester.runAsync(() async {
      final saved = HttpOverrides.current;
      HttpOverrides.global = null;
      try {
        final btn = tester.widget<IconButton>(find.ancestor(
            of: find.byIcon(Icons.refresh),
            matching: find.byType(IconButton)));
        btn.onPressed!();
        for (var i = 0; i < 50 && upgrades.isEmpty; i++) {
          await Future<void>.delayed(const Duration(milliseconds: 100));
        }
      } finally {
        HttpOverrides.global = saved as HttpOverrides?;
      }
    });
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));

    // 首轮消耗 tk-1（fake zone 未完成握手），重连用 tk-2 完成真 upgrade，
    // 票据作 WS 子协议被携带
    expect(upgrades, isNotEmpty);
    expect(upgrades.last, 'tk-2');
    expect(find.byIcon(Icons.error_outline), findsNothing);

    // 服务端下发 close 帧 → 错误横幅显示「连接已关闭」
    await tester.runAsync(() async {
      sessions.last.add(jsonEncode({'type': 'close'}));
      await Future<void>.delayed(const Duration(milliseconds: 200));
    });
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));
    // _fail 对 connected 态的服务端正常 close 只置错误态、不覆盖文案（空横幅）
    expect(find.byIcon(Icons.error_outline), findsOneWidget);

    await tester.runAsync(() => server.close());
  });
}
