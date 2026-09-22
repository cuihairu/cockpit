import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/pages/login_page.dart';
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

class _MemSettings extends SettingsNotifier {
  String? token;

  @override
  SettingsState build() =>
      SettingsState(serverUrl: 'http://test', loaded: true);

  @override
  Future<String?> readToken() async => token;
  @override
  Future<void> saveToken(String t) async => token = t;
  @override
  Future<void> clearToken() async => token = null;
}

CockpitApi _api(MockAdapter a) => CockpitApi(ApiClient.forTest(
    Dio(BaseOptions(baseUrl: 'http://test'))..httpClientAdapter = a));

Future<ProviderContainer> _pump(WidgetTester tester, MockAdapter a,
    {Object? apiError}) async {
  final c = ProviderContainer(overrides: [
    settingsProvider.overrideWith(() => _MemSettings()),
    apiProvider.overrideWith((ref) async {
      if (apiError != null) throw apiError;
      return _api(a);
    }),
  ]);
  addTearDown(c.dispose);
  await tester.pumpWidget(UncontrolledProviderScope(
    container: c,
    child: const MaterialApp(home: LoginPage()),
  ));
  return c;
}

void main() {
  testWidgets('登录页：账密提交成功 → Authenticated', (tester) async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 200,
          {'token': 'jwt-1', 'username': 'alice', 'role': 'admin'});
    final c = await _pump(tester, a);

    await tester.enterText(
        find.widgetWithText(TextField, '用户名'), 'alice');
    await tester.enterText(find.widgetWithText(TextField, '密码'), 'pw');
    await tester.tap(find.widgetWithText(FilledButton, '登录'));
    await tester.pumpAndSettle();

    expect(c.read(authProvider), isA<Authenticated>());
    expect((c.read(settingsProvider.notifier) as _MemSettings).token, 'jwt-1');
  });

  testWidgets('登录页：401 带错误体 → 展示后端 error 文案', (tester) async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 401, {'error': '用户名或密码错误'});
    await _pump(tester, a);

    await tester.enterText(find.widgetWithText(TextField, '用户名'), 'a');
    await tester.enterText(find.widgetWithText(TextField, '密码'), 'b');
    await tester.tap(find.widgetWithText(FilledButton, '登录'));
    await tester.pumpAndSettle();

    expect(find.text('用户名或密码错误'), findsOneWidget);
  });

  testWidgets('登录页：API 栈异常 → 展示 toString', (tester) async {
    await _pump(tester, MockAdapter(), apiError: StateError('boom'));

    await tester.enterText(find.widgetWithText(TextField, '用户名'), 'a');
    await tester.enterText(find.widgetWithText(TextField, '密码'), 'b');
    await tester.tap(find.widgetWithText(FilledButton, '登录'));
    await tester.pumpAndSettle();

    expect(find.textContaining('Bad state: boom'), findsOneWidget);
  });

  testWidgets('登录页：密码框回车提交（onSubmitted）', (tester) async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 200,
          {'token': 'jwt-2', 'username': 'bob', 'role': ''});
    final c = await _pump(tester, a);

    await tester.enterText(find.widgetWithText(TextField, '用户名'), 'bob');
    final pw = find.widgetWithText(TextField, '密码');
    await tester.enterText(pw, 'pw');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();

    expect(c.read(authProvider), isA<Authenticated>());
  });

  testWidgets('登录页：二步验证切换与返回', (tester) async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 200,
          {'requires_totp': true, 'tmp_token': 'tmp-1'});
    final c = await _pump(tester, a);

    await tester.enterText(find.widgetWithText(TextField, '用户名'), 'alice');
    await tester.enterText(find.widgetWithText(TextField, '密码'), 'pw');
    await tester.tap(find.widgetWithText(FilledButton, '登录'));
    await tester.pumpAndSettle();

    expect(find.text('两步验证'), findsOneWidget);
    expect(find.widgetWithText(TextField, '验证码'), findsOneWidget);
    expect(c.read(authProvider), isA<AuthTotpRequired>());

    await tester.tap(find.text('返回'));
    await tester.pumpAndSettle();
    // 回到账密步：AppBar 标题与提交按钮均为「登录」
    expect(find.text('登录'), findsNWidgets(2));
    expect(find.widgetWithText(TextField, '用户名'), findsOneWidget);
    expect(c.read(authProvider), isA<AuthAnonymous>());
  });

  testWidgets('登录页：TOTP 验证码提交（含回车）成功', (tester) async {
    final a = MockAdapter()
      ..on('POST', '/api/auth/login', 200,
          {'requires_totp': true, 'tmp_token': 'tmp-1'})
      ..on('POST', '/api/auth/totp/verify', 200,
          {'token': 'jwt-3', 'username': 'alice', 'role': 'admin'});
    final c = await _pump(tester, a);

    await tester.enterText(find.widgetWithText(TextField, '用户名'), 'alice');
    await tester.enterText(find.widgetWithText(TextField, '密码'), 'pw');
    await tester.tap(find.widgetWithText(FilledButton, '登录'));
    await tester.pumpAndSettle();

    await tester.enterText(find.widgetWithText(TextField, '验证码'), '123456');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pumpAndSettle();

    expect(c.read(authProvider), isA<Authenticated>());
  });
}
