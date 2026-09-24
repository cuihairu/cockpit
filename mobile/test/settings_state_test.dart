import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:cockpit_mobile/state/auth.dart';
import 'package:cockpit_mobile/state/settings.dart';

const _storageChannel =
    MethodChannel('plugins.it_nomads.com/flutter_secure_storage');

/// channel 层 fake secure storage：内存 Map。
void _mockSecureStorage(Map<String, String> store) {
  TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
      .setMockMethodCallHandler(_storageChannel, (call) async {
    switch (call.method) {
      case 'read':
        return store[call.arguments['key'] as String];
      case 'write':
        store[call.arguments['key'] as String] =
            call.arguments['value'] as String;
        return null;
      case 'delete':
        store.remove(call.arguments['key'] as String);
        return null;
      default:
        return null;
    }
  });
}

/// 驱动 container 的调度与 _load 异步链直到 settings 就绪。
Future<void> _settle(ProviderContainer c) async {
  for (var i = 0; i < 10 && !c.read(settingsProvider).loaded; i++) {
    await c.pump();
    await Future<void>.delayed(Duration.zero);
  }
  await c.pump();
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    SharedPreferences.setMockInitialValues({
      'server_url': 'http://initial',
      'allow_self_signed': true,
      'biometric_lock': true,
    });
  });

  tearDown(() {
    TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_storageChannel, null);
  });

  test('SettingsNotifier：_load 读出三键 + copyWith', () async {
    final store = <String, String>{};
    _mockSecureStorage(store);
    final c = ProviderContainer();
    c.read(settingsProvider.notifier);
    // build() 里 _load 异步，等一拍
    await Future<void>.delayed(Duration.zero);
    final state = c.read(settingsProvider);
    expect(state.loaded, isTrue);
    expect(state.serverUrl, 'http://initial');
    expect(state.allowSelfSigned, isTrue);
    expect(state.biometricLock, isTrue);

    final copied = state.copyWith(allowSelfSigned: false);
    expect(copied.allowSelfSigned, isFalse);
    expect(copied.serverUrl, 'http://initial');
    expect(state.configured, isTrue);

    c.dispose();
  });

  test('SettingsNotifier：三个 setter 落盘并广播', () async {
    final store = <String, String>{};
    _mockSecureStorage(store);
    final c = ProviderContainer();
    final s = c.read(settingsProvider.notifier);
    await Future<void>.delayed(Duration.zero);

    await s.setServerUrl('http://next');
    await s.setAllowSelfSigned(false);
    await s.setBiometricLock(false);
    final state = c.read(settingsProvider);
    expect(state.serverUrl, 'http://next');
    expect(state.allowSelfSigned, isFalse);
    expect(state.biometricLock, isFalse);

    final prefs = await SharedPreferences.getInstance();
    expect(prefs.getString('server_url'), 'http://next');
    expect(prefs.getBool('allow_self_signed'), isFalse);
    c.dispose();
  });

  test('SettingsNotifier：token 走 secure storage（read/write/clear）', () async {
    final store = <String, String>{};
    _mockSecureStorage(store);
    final c = ProviderContainer();
    final s = c.read(settingsProvider.notifier);
    await Future<void>.delayed(Duration.zero);

    expect(await s.readToken(), isNull);
    await s.saveToken('jwt-1');
    expect(await s.readToken(), 'jwt-1');
    expect(store['auth_token'], 'jwt-1');
    await s.clearToken();
    expect(await s.readToken(), isNull);
    c.dispose();
  });

  test('apiProvider：未配置 server 抛 ServerNotConfigured', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final c = ProviderContainer();
    await _settle(c);
    final sub = c.listen(apiProvider, (_, _) {});
    await c.pump();
    final state = c.read(apiProvider);
    expect(state.hasError, isTrue, reason: '未配置应进入错误态');
    expect(state.error, isA<ServerNotConfigured>());
    sub.close();
    c.dispose();
  });

  test('apiProvider：已配置 → 真实 ApiClient.create 成链', () async {
    _mockSecureStorage(<String, String>{'auth_token': 'jwt-9'});
    final c = ProviderContainer();
    await _settle(c);
    final api = await c.read(apiProvider.future);
    expect(api.client.dio.options.baseUrl, 'http://initial');
    // create 链路带上持久化 token
    expect(api.client.dio.interceptors, isNotEmpty);
    c.dispose();
  });

  test('FlutterSecureStorage fake 通道可用性自检', () async {
    _mockSecureStorage(<String, String>{});
    expect(await const FlutterSecureStorage().read(key: 'x'), isNull);
    await const FlutterSecureStorage().write(key: 'x', value: 'y');
    expect(await const FlutterSecureStorage().read(key: 'x'), 'y');
  });

  test('apiProvider onUnauthorized：401 且刷新失败 → 清 token 并登出', () async {
    // 全 401 的真 server：原请求 401 → refresh 401 → onUnauthorized 回调。
    // flutter_test 的 HttpOverrides 会把真请求一律劫持成 400，先还原。
    final savedOverrides = HttpOverrides.current;
    HttpOverrides.global = null;
    final server = await HttpServer.bind('127.0.0.1', 0);
    server.listen((req) async {
      req.response.statusCode = 401;
      await req.response.close();
    });
    SharedPreferences.setMockInitialValues({
      'server_url': 'http://127.0.0.1:${server.port}',
      'allow_self_signed': false,
    });
    final store = <String, String>{'auth_token': 'jwt-1'};
    _mockSecureStorage(store);
    final c = ProviderContainer();
    try {
      await _settle(c);
      final api = await c.read(apiProvider.future);
      await expectLater(
          api.client.dio.get('/api/agents'), throwsA(isA<DioException>()));
      expect(store['auth_token'], isNull, reason: 'onUnauthorized 应清 token');
      expect(c.read(authProvider), isA<AuthAnonymous>(), reason: '应回未登录态');
    } finally {
      c.dispose();
      await server.close();
      HttpOverrides.global = savedOverrides;
    }
  });

  test('apiProvider 401 → refresh 成功 → saveToken 并重放成功', () async {
    // 原请求带旧 token 收 401 → refresh 发新 token → saveToken 落盘 →
    // 原请求带新 token 重放成功。服务端按 Authorization 头区分。
    final savedOverrides = HttpOverrides.current;
    HttpOverrides.global = null;
    final server = await HttpServer.bind('127.0.0.1', 0);
    server.listen((req) async {
      final auth = req.headers.value('Authorization');
      req.response.headers.contentType = ContentType.json;
      if (req.method == 'POST' && req.uri.path == '/api/auth/refresh') {
        req.response.statusCode = 200;
        req.response.write('{"token":"jwt-new"}');
        await req.response.close();
        return;
      }
      if (auth == 'Bearer jwt-new') {
        req.response.statusCode = 200;
        req.response.write('[]');
      } else {
        req.response.statusCode = 401;
        req.response.write('{"error":"expired"}');
      }
      await req.response.close();
    });
    SharedPreferences.setMockInitialValues({
      'server_url': 'http://127.0.0.1:${server.port}',
      'allow_self_signed': false,
    });
    final store = <String, String>{'auth_token': 'jwt-1'};
    _mockSecureStorage(store);
    final c = ProviderContainer();
    try {
      await _settle(c);
      final api = await c.read(apiProvider.future);
      // 原请求 401 → refresh → 重放 200，无异常冒出
      final agents = await api.agents();
      expect(agents, isEmpty);
      expect(store['auth_token'], 'jwt-new',
          reason: 'onTokenRefreshed 应 saveToken 落盘');
    } finally {
      c.dispose();
      await server.close();
      HttpOverrides.global = savedOverrides;
    }
  });
}
