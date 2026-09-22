import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/state/auth.dart';
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

/// 内存版 settings：token 存取 + 可配置 serverUrl。
class _MemSettings extends SettingsNotifier {
  _MemSettings({this.url = 'http://test'});

  final String url;
  String? token;

  @override
  SettingsState build() => SettingsState(serverUrl: url, loaded: true);

  @override
  Future<String?> readToken() async => token;

  @override
  Future<void> saveToken(String t) async => token = t;

  @override
  Future<void> clearToken() async => token = null;
}

Future<ProviderContainer> _container(
    {required MockAdapter adapter, String url = 'http://test'}) async {
  final c = ProviderContainer(overrides: [
    settingsProvider.overrideWith(() => _MemSettings(url: url)),
    apiProvider.overrideWith((ref) async => CockpitApi(ApiClient.forTest(
        Dio(BaseOptions(baseUrl: url))..httpClientAdapter = adapter))),
  ]);
  return c;
}

void main() {
  test('bootstrap：未配置 → Unconfigured；无 token → Anonymous；有 token → Authenticated',
      () async {
    final a = MockAdapter();
    final c = await _container(adapter: a, url: '');
    final auth = c.read(authProvider.notifier);
    await auth.bootstrap();
    expect(c.read(authProvider), isA<AuthUnconfigured>());

    final c2 = await _container(adapter: a);
    final auth2 = c2.read(authProvider.notifier);
    await auth2.bootstrap();
    expect(c2.read(authProvider), isA<AuthAnonymous>());

    // 有 token：直接恢复为已登录（username 空待拉取）
    final c3 = await _container(adapter: a);
    final mem = c3.read(settingsProvider.notifier) as _MemSettings;
    mem.token = 'jwt-1';
    final auth3 = c3.read(authProvider.notifier);
    await auth3.bootstrap();
    final s3 = c3.read(authProvider);
    expect(s3, isA<Authenticated>());
    expect((s3 as Authenticated).username, '');
  });

  test('login：requiresTotp → TotpRequired；verifyTotp → Authenticated + 落盘',
      () async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 200,
          {'requires_totp': true, 'tmp_token': 'tmp-1'})
      ..on('POST', '/api/auth/totp/verify', 200,
          {'token': 'jwt-2', 'username': 'alice', 'role': 'admin'});
    final c = await _container(adapter: a);
    final auth = c.read(authProvider.notifier);

    await auth.login('alice', 'pw');
    AuthState s = c.read(authProvider);
    expect(s, isA<AuthTotpRequired>());
    expect((s as AuthTotpRequired).tmpToken, 'tmp-1');

    // 非 TotpRequired 状态下 verifyTotp 是 no-op
    c.read(authProvider.notifier).state = const AuthAnonymous();
    await c.read(authProvider.notifier).verifyTotp('000000');
    expect(c.read(authProvider), isA<AuthAnonymous>());

    // 正常二步
    c.read(authProvider.notifier).state = AuthTotpRequired('tmp-1');
    await c.read(authProvider.notifier).verifyTotp('123456');
    s = c.read<AuthState>(authProvider);
    expect(s, isA<Authenticated>());
    expect((s as Authenticated).username, 'alice');
    expect((c.read(settingsProvider.notifier) as _MemSettings).token, 'jwt-2');
  });

  test('login：无 TOTP 直接成功 → 落盘 + Authenticated', () async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 200,
          {'token': 'jwt-3', 'username': 'bob', 'role': 'viewer'});
    final c = await _container(adapter: a);
    await c.read(authProvider.notifier).login('bob', 'pw');
    final s = c.read(authProvider);
    expect(s, isA<Authenticated>());
    expect((s as Authenticated).role, 'viewer');
  });

  test('cancelTotp/logout/signedOut：回到 Anonymous 且清 token', () async {
    final a = MockAdapter();
    final c = await _container(adapter: a);
    final mem = c.read(settingsProvider.notifier) as _MemSettings;
    final auth = c.read(authProvider.notifier);

    auth.state = const AuthTotpRequired('t');
    auth.cancelTotp();
    expect(c.read(authProvider), isA<AuthAnonymous>());

    mem.token = 'jwt-x';
    await auth.logout();
    expect(mem.token, isNull);
    expect(c.read(authProvider), isA<AuthAnonymous>());

    auth.signedOut();
    expect(c.read(authProvider), isA<AuthAnonymous>());
  });
}
