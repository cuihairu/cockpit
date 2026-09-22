import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';

/// 可编程 mock adapter：按 (method, path) 注册响应，记录每次请求的 Authorization 头。
class MockAdapter implements HttpClientAdapter {
  /// 同一 (method, path) 的响应按注册顺序依次弹出，支持 401→200 序列。
  final responses = <String, List<ResponseBody>>{};
  final authHeaders = <String?>[];

  void on(String method, String path, int status, Object? body) {
    responses.putIfAbsent('$method $path', () => []).add(
          ResponseBody.fromString(
            jsonEncode(body),
            status,
            headers: {
              Headers.contentTypeHeader: [Headers.jsonContentType],
            },
          ),
        );
  }

  @override
  Future<ResponseBody> fetch(
      RequestOptions options,
      Stream<Uint8List>? requestStream,
      Future<void>? cancelFuture) async {
    final key = '${options.method} ${options.path}';
    authHeaders.add(options.headers['Authorization'] as String?);
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

void main() {
  group('ApiClient', () {
    test('baseUrl 规范化去尾斜杠', () async {
      final c = await ApiClient.create(
          baseUrl: 'https://cockpit.example.com/',
          allowSelfSigned: false);
      expect(c.dio.options.baseUrl, 'https://cockpit.example.com');
    });

    test('请求自动附带 Authorization 头', () async {
      final adapter = MockAdapter()
        ..on('GET', '/api/agents', 200, <Object?>[]);
      final dio = Dio(BaseOptions(baseUrl: 'http://x'))
        ..httpClientAdapter = adapter;
      final client = ApiClient.forTest(dio)..updateToken('jwt-1');
      final r = await client.dio.get<List<dynamic>>('/api/agents');
      expect(r.statusCode, 200);
      expect(adapter.authHeaders.single, 'Bearer jwt-1');
    });

    test('401 → 刷新 → 原请求以新 token 重放一次', () async {
      final adapter = MockAdapter()
        ..on('GET', '/api/agents', 401, {'error': 'expired'})
        ..on('POST', '/api/auth/refresh', 200, {'token': 'jwt-2'})
        ..on('GET', '/api/agents', 200, [
          {'id': 'ag-1'}
        ]);
      final dio = Dio(BaseOptions(baseUrl: 'http://x'))
        ..httpClientAdapter = adapter;
      String? refreshed;
      final client = ApiClient.createWith(
          dio: dio,
          initialToken: 'jwt-1',
          onTokenRefreshed: (t) => refreshed = t);
      final r = await client.dio.get<List<dynamic>>('/api/agents');
      expect(r.statusCode, 200);
      expect(refreshed, 'jwt-2');
      // 请求序列：初 401（旧头）→ refresh（携带旧头，后端据其识别会话）→ 重放（新头）
      expect(
          adapter.authHeaders.where((h) => h == 'Bearer jwt-1').length, 2);
      expect(
          adapter.authHeaders.where((h) => h == 'Bearer jwt-2').length, 1);
    });

    test('刷新失败 → 回调 onUnauthorized（登出）且异常上抛', () async {
      final adapter = MockAdapter()
        ..on('GET', '/api/agents', 401, {'error': 'expired'})
        ..on('POST', '/api/auth/refresh', 401, {'error': 'expired too'});
      final dio = Dio(BaseOptions(baseUrl: 'http://x'))
        ..httpClientAdapter = adapter;
      var unauthorized = false;
      final client = ApiClient.createWith(
          dio: dio,
          initialToken: 'jwt-1',
          onUnauthorized: () async => unauthorized = true);
      await expectLater(
          client.dio.get('/api/agents'), throwsA(isA<DioException>()));
      expect(unauthorized, isTrue);
    });
  });
}
