import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:xterm/xterm.dart';

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

/// 挂起到 gate 放行才返回 ticket 响应的 adapter。
class _GatedAdapter implements HttpClientAdapter {
  _GatedAdapter(this.gate);

  final Completer<void> gate;

  @override
  Future<ResponseBody> fetch(RequestOptions options,
      Stream<Uint8List>? requestStream, Future<void>? cancelFuture) async {
    await gate.future;
    return ResponseBody.fromString(
        jsonEncode(
            {'ticket': 'tk-9', 'expires_at': '2030-01-01T00:00:00Z'}),
        200,
        headers: {
          Headers.contentTypeHeader: [Headers.jsonContentType],
        });
  }

  @override
  void close({bool force = false}) {}
}

class _UrlSettings extends SettingsNotifier {
  _UrlSettings(this.url, {this.selfSigned = false});

  final String url;
  final bool selfSigned;

  @override
  SettingsState build() =>
      SettingsState(serverUrl: url, allowSelfSigned: selfSigned, loaded: true);
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
        HttpOverrides.global = saved;
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
        HttpOverrides.global = saved;
      }
    });
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));

    // 首轮消耗 tk-1（fake zone 未完成握手），重连用 tk-2 完成真 upgrade，
    // 票据作 WS 子协议被携带
    expect(upgrades, isNotEmpty);
    expect(upgrades.last, 'tk-2');
    expect(find.byIcon(Icons.error_outline), findsNothing);

    // 键盘输出回调 → input 消息发往服务端（onOutput 转发链）
    await tester.runAsync(() async {
      final tv = tester.widget<TerminalView>(find.byType(TerminalView));
      tv.terminal.onOutput!('ls\r\n');
      await Future<void>.delayed(const Duration(milliseconds: 200));
    });
    expect(
        inputs.any((m) => m.contains('"type":"input"') && m.contains('ls')),
        isTrue);

    // 服务端下发 close 帧 → 错误横幅（真 zone 消息派发需多等几拍）。
    // 首轮 fake-zone 握手被推迟到真 zone：tk-2 先被 accept，活跃连接是
    // sessions.first（sessions.last 是 refresh 时已关闭的 tk-1 连接）。
    await tester.runAsync(() async {
      sessions.first.add(jsonEncode({'type': 'close'}));
      await Future<void>.delayed(const Duration(milliseconds: 800));
    });
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));
    await tester.pump(const Duration(milliseconds: 200));
    // _fail 对 connected 态的服务端正常 close 只置错误态、不覆盖文案（空横幅）
    expect(find.byIcon(Icons.error_outline), findsOneWidget);

    // 主动卸载 widget → dispose 触发 sink.close()，后者挂 5s close 超时 Timer；
    // 推进 fake time 让其落定，否则 _verifyInvariants 报 timersPending。
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 5));
    await tester.runAsync(() => server.close());
  });

  testWidgets('终端页：ticket 返回前页面已退出 → 放弃连接并关闭 channel',
      (tester) async {
    // 门控 adapter：tickets 请求挂起，页面卸载后放行 → !mounted 分支
    final gate = Completer<void>();
    final adapter = _GatedAdapter(gate);
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _UrlSettings('http://127.0.0.1:9')),
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
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));

    // 认证期间退出页面；放行 ticket 后 _connect 恢复 → mounted=false →
    // 关闭刚打开的 channel，不 setState
    await tester.pumpWidget(const SizedBox());
    gate.complete();
    // sink.close 的超时 Timer 需要推进 fake 时间才能清干净
    await tester.pump(const Duration(seconds: 6));
    await tester.pump(const Duration(milliseconds: 200));
  });

  testWidgets('终端页：https 自签 → wss 分支连通', (tester) async {
    final ctx = SecurityContext()
      ..useCertificateChain(
          '${Directory.current.path}/test/fixtures/localhost-cert.pem')
      ..usePrivateKey(
          '${Directory.current.path}/test/fixtures/localhost-key.pem');
    final upgrades2 = <String?>[];
    final server = (await tester.runAsync(() async {
      final saved = HttpOverrides.current;
      HttpOverrides.global = null;
      try {
        final s = await HttpServer.bindSecure('127.0.0.1', 0, ctx);
        s.listen((req) async {
          final ws = await WebSocketTransformer.upgrade(req,
              protocolSelector: (protocols) => protocols.first);
          upgrades2.add(ws.protocol);
          ws.listen((_) {});
        });
        return s;
      } finally {
        HttpOverrides.global = saved;
      }
    }))!;

    // 首轮 tickets 不注册（404）→ 页面停在错误态且未创建 channel，
    // 避免 fake zone 的 mock 握手留下无人处理的 WS error
    final adapter = MockAdapter();
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter;
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() =>
            _UrlSettings('https://127.0.0.1:${server.port}',
                selfSigned: true)),
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
    // 首轮 ticket 404 收尾（fake zone 微任务即可完成）
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));

    // 注入 refresh 用的 ticket 响应（首轮 404 已被消费为错误态）
    adapter.on('POST', '/api/remote/tickets', 200,
        {'ticket': 'tk-wss', 'expires_at': '2030-01-01T00:00:00Z'});

    // 重连触发真 wss 握手（allowSelfSigned → badCertificateCallback 放行；
    // customClient 绕过 flutter_test 的 HttpOverrides，需留在真 zone）
    await tester.runAsync(() async {
      final saved = HttpOverrides.current;
      HttpOverrides.global = null;
      try {
        final btn = tester.widget<IconButton>(find.ancestor(
            of: find.byIcon(Icons.refresh),
            matching: find.byType(IconButton)));
        btn.onPressed!();
        for (var i = 0; i < 50 && upgrades2.isEmpty; i++) {
          await Future<void>.delayed(const Duration(milliseconds: 100));
        }
      } finally {
        HttpOverrides.global = saved;
      }
    });
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));

    expect(upgrades2, isNotEmpty);
    expect(upgrades2.last, 'tk-wss');

    await tester.runAsync(() => server.close());
  });

  testWidgets('终端页：WS 协议帧非法 → stream onError 显示连接错误文案',
      (tester) async {
    // 升级前持有 raw socket，升级响应后紧跟保留 opcode（0x3）帧字节：
    // RFC 6455 §5.2 要求客户端收到保留 opcode 即 fail connection，
    // dart:io 协议解析抛 WebSocketException → channel.stream onError
    // （此前登记 KNOWN_UNCOVERABLE 的 onError 分支由此路径真实触发）。
    final upgraded = Completer<void>();
    final server = (await tester.runAsync(() async {
      final saved = HttpOverrides.current;
      HttpOverrides.global = null;
      try {
        final s = await HttpServer.bind('127.0.0.1', 0);
        s.listen((req) async {
          final raw = req.socket;
          final ws = await WebSocketTransformer.upgrade(req,
              protocolSelector: (protocols) => protocols.first);
          raw.add([0x83, 0x01, 0x78]); // FIN + 保留 opcode 3，负载 'x'
          upgraded.complete();
          ws.listen((_) {});
        });
        return s;
      } finally {
        HttpOverrides.global = saved;
      }
    }))!;

    final adapter = MockAdapter()
      ..on('POST', '/api/remote/tickets', 200,
          {'ticket': 'tk-err', 'expires_at': '2030-01-01T00:00:00Z'});
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
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));

    // 真 async 区等握手完成 + 客户端解析非法帧 → onError → _fail
    await tester.runAsync(() async {
      for (var i = 0; i < 50 && !upgraded.isCompleted; i++) {
        await Future<void>.delayed(const Duration(milliseconds: 100));
      }
      await Future<void>.delayed(const Duration(milliseconds: 800));
    });
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 200));
    await tester.pump(const Duration(milliseconds: 200));
    // onError（写文案 + 置 error 态）先于 onDone（connected 态下不写文案）；
    // 若只有 onDone，L123 条件不满足、横幅为空——文案出现即 onError 已执行
    expect(find.byIcon(Icons.error_outline), findsOneWidget);
    expect(find.textContaining('连接已关闭'), findsOneWidget);

    // 卸载 → dispose → sink.close() 挂 5s close 超时 Timer，推进 fake time
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 5));
    await tester.runAsync(() => server.close(force: true));
  });

}
