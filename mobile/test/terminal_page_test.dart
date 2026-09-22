import 'dart:convert';
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
        settingsProvider.overrideWith(() => _FakeSettings()),
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
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}
