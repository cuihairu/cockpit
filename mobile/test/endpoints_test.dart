import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';

class MockAdapter implements HttpClientAdapter {
  final responses = <String, List<ResponseBody>>{};
  final requests = <String, Object?>{};

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
    requests[key] = {
      'query': options.queryParameters,
      'data': options.data,
    };
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

CockpitApi _api(MockAdapter adapter) =>
    CockpitApi(ApiClient.forTest(Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter));

void main() {
  test('login：POST body 与解析', () async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 200, {
        'token': 'jwt-1',
        'user_id': '1',
        'username': 'alice',
        'role': 'admin',
      });
    final api = _api(a);
    final r = await api.login('alice', 'pw');
    expect(r.token, 'jwt-1');
    expect(r.username, 'alice');
    expect(r.requiresTotp, isFalse);
    expect(
        (a.requests['POST /api/auth/login'] as Map)['data'],
        {'username': 'alice', 'password': 'pw'});
  });

  test('verifyTotp：POST tmp_token 与解析', () async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/totp/verify', 200,
          {'token': 'jwt-2', 'username': 'bob', 'role': 'viewer'});
    final r = await _api(a).verifyTotp('123456', 'tmp-1');
    expect(r.token, 'jwt-2');
    expect((a.requests['POST /api/auth/totp/verify'] as Map)['data'],
        {'code': '123456', 'tmp_token': 'tmp-1'});
  });

  test('agents/alerts/readAll/containers：方法路径与形状', () async {
    final a = MockAdapter()
      ..on('GET', '/api/agents', 200, [
        {
          'id': 'ag-1',
          'hostname': 'h1',
          'ip': '1.1.1.1',
          'status': 'online',
          'capabilities': [
            {'type': 'docker-api'}
          ],
        }
      ])
      ..on('GET', '/api/alerts', 200, {
        'data': [
          {'id': 'al-1', 'type': 'error', 'title': 't', 'message': 'm'}
        ]
      })
      ..on('PUT', '/api/alerts/read-all', 200, {})
      ..on('GET', '/api/docker/agents/ag-1/containers', 200, [
        {'ID': 'c1', 'Name': '/web', 'Image': 'nginx', 'State': 'running',
         'Status': 'Up', 'Created': 1758500000}
      ]);
    final api = _api(a);

    final agents = await api.agents();
    expect(agents, hasLength(1));
    expect(agents.first.hasDocker, isTrue);

    final alerts = await api.alerts();
    expect(alerts.first.type, 'error');
    expect(alerts.first.read, isFalse);

    await api.readAllAlerts();
    expect(a.requests.containsKey('PUT /api/alerts/read-all'), isTrue);

    final cs = await api.containers('ag-1');
    expect(cs.first.name, '/web'); // Docker 原始 Name 保留前导斜杠
    expect((a.requests['GET /api/docker/agents/ag-1/containers'] as Map)['query'],
        {'all': true});
  });

  test('containerAction：白名单外抛 ArgumentError', () async {
    final a = MockAdapter();
    expect(() => _api(a).containerAction('ag', 'c1', 'rm'),
        throwsArgumentError);
    expect(a.requests, isEmpty);
  });

  test('containerAction：白名单内放行', () async {
    final a = MockAdapter()
      ..on('POST', '/api/docker/agents/ag-1/containers/c1/restart', 200, {});
    await _api(a).containerAction('ag-1', 'c1', 'restart');
    expect(a.requests.containsKey('POST /api/docker/agents/ag-1/containers/c1/restart'),
        isTrue);
  });

  test('status：GET /api/status 聚合解析', () async {
    final a = MockAdapter()
      ..on('GET', '/api/status', 200, {
        'infrastructure': {'total': 3, 'online': 2},
        'domains': {'valid': 5, 'expiring': 1},
        'certificates': {'valid': 2, 'expiring': 0},
        'services': {'down': 4},
      });
    final s = await _api(a).status();
    expect(s.agentsTotal, 3);
    expect(s.agentsOnline, 2);
    expect(s.domainsExpiring, 1);
    expect(s.servicesDown, 4);
  });

  test('createRemoteTicket：带/不带凭据的 body 形状', () async {
    final a = MockAdapter()
      ..on('POST', '/api/remote/tickets', 200,
          {'ticket': 'tk-1', 'expires_at': '2026-09-22T00:00:00Z'})
      ..on('POST', '/api/remote/tickets', 200, {'ticket': 'tk-2'});
    final api = _api(a);

    final t1 = await api.createRemoteTicket(
        agentId: 'ag-1', host: 'h', port: 22,
        username: 'root', password: 'pw');
    expect(t1.ticket, 'tk-1');
    expect((a.requests['POST /api/remote/tickets'] as Map)['data'], {
      'agent_id': 'ag-1',
      'host': 'h',
      'port': 22,
      'protocol': 'ssh',
      'username': 'root',
      'password': 'pw',
    });

    final t2 = await api.createRemoteTicket(agentId: 'ag-1', host: 'h', port: 22);
    expect(t2.expiresAt, '');
    final body2 = (a.requests['POST /api/remote/tickets'] as Map)['data']
        as Map<String, dynamic>;
    expect(body2.containsKey('username'), isFalse);
    expect(body2.containsKey('password'), isFalse);
  });
}
