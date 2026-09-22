import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/pages/settings_page.dart';
import 'package:cockpit_mobile/state/auth.dart';
import 'package:cockpit_mobile/state/biometric.dart';
import 'package:cockpit_mobile/state/settings.dart';

class _FakeBiometric implements BiometricAuth {
  _FakeBiometric({required this.supported});

  final bool supported;

  @override
  Future<bool> isSupported() async => supported;

  @override
  Future<bool> authenticate(String reason) async => true;
}

class _FakeSettings extends SettingsNotifier {
  @override
  SettingsState build() =>
      const SettingsState(serverUrl: 'http://test', loaded: true);
}

/// 可写 settings：记录开关写入，不落盘。
class _WritableSettings extends SettingsNotifier {
  _WritableSettings({this.biometricLock = false});

  final bool biometricLock;
  bool? savedSelfSigned;
  bool? savedBiometric;

  @override
  SettingsState build() => SettingsState(
      serverUrl: 'http://test',
      allowSelfSigned: true,
      biometricLock: biometricLock,
      loaded: true);

  @override
  Future<void> setAllowSelfSigned(bool v) async => savedSelfSigned = v;

  @override
  Future<void> setBiometricLock(bool v) async => savedBiometric = v;
}

class _FakeAuth extends AuthNotifier {
  _FakeAuth(this._initial);

  final AuthState _initial;
  int logouts = 0;

  @override
  AuthState build() => _initial;

  @override
  Future<void> logout() async => logouts++;
}

Future<void> _pump(WidgetTester tester, BiometricAuth bio) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => _FakeSettings()),
      authProvider.overrideWith(() => _FakeAuth(const AuthAnonymous())),
      biometricProvider.overrideWithValue(bio),
    ],
    child: const MaterialApp(home: Scaffold(body: SettingsPage())),
  ));
}

class _MockAdapter implements HttpClientAdapter {
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
    final queue =
        responses['${options.method} ${options.path}'];
    if (queue == null || queue.isEmpty) {
      return ResponseBody.fromString(
          jsonEncode({'error': 'no route'}), 404,
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
  testWidgets('生物识别锁开关：设备支持时可用', (tester) async {
    await _pump(tester, _FakeBiometric(supported: true));
    await tester.pumpAndSettle();

    expect(find.text('启动生物识别锁'), findsOneWidget);
    expect(find.text('打开应用需验证指纹或面容'), findsOneWidget);
    final switchTile = tester.widget<SwitchListTile>(
        find.widgetWithText(SwitchListTile, '启动生物识别锁'));
    expect(switchTile.onChanged, isNotNull);
  });

  testWidgets('生物识别锁开关：设备不支持时禁用', (tester) async {
    await _pump(tester, _FakeBiometric(supported: false));
    await tester.pumpAndSettle();

    expect(find.text('本设备不支持生物识别'), findsOneWidget);
    final switchTile = tester.widget<SwitchListTile>(
        find.widgetWithText(SwitchListTile, '启动生物识别锁'));
    expect(switchTile.onChanged, isNull);
  });

  testWidgets('设置页：Server 入口提示退出重配、自签与生物锁开关落盘、退出登录',
      (tester) async {
    final settings = _WritableSettings(biometricLock: false);
    final auth = _FakeAuth(const AuthAnonymous());
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => settings),
        authProvider.overrideWith(() => auth),
        biometricProvider.overrideWithValue(_FakeBiometric(supported: true)),
      ],
      child: const MaterialApp(home: Scaffold(body: SettingsPage())),
    ));
    await tester.pumpAndSettle();

    // Server tile → SnackBar 提示
    await tester.tap(find.text('Server'));
    await tester.pumpAndSettle();
    expect(find.textContaining('修改地址请退出登录后在引导页重新配置'), findsOneWidget);

    // 自签开关 → setAllowSelfSigned(false)
    await tester.tap(find.widgetWithText(SwitchListTile, '允许自签证书'));
    await tester.pumpAndSettle();
    expect(settings.savedSelfSigned, isFalse);

    // 生物锁开关 → setBiometricLock(true)
    await tester.tap(find.widgetWithText(SwitchListTile, '启动生物识别锁'));
    await tester.pumpAndSettle();
    expect(settings.savedBiometric, isTrue);

    // 退出登录
    await tester.tap(find.text('退出登录'));
    await tester.pumpAndSettle();
    expect(auth.logouts, 1);
  });

  testWidgets('设置页：审计日志入口跳转', (tester) async {
    final adapter = _MockAdapter()
      ..on('GET', '/api/admin/audit/logs', 200,
          {'data': <Object?>[], 'pagination': {'total': 0}});
    final api = CockpitApi(ApiClient.forTest(
        Dio(BaseOptions(baseUrl: 'http://test'))..httpClientAdapter = adapter));
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        authProvider.overrideWith(() => _FakeAuth(const AuthAnonymous())),
        biometricProvider.overrideWithValue(_FakeBiometric(supported: false)),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const MaterialApp(home: Scaffold(body: SettingsPage())),
    ));
    await tester.pumpAndSettle();

    await tester.tap(find.text('审计日志'));
    await tester.pumpAndSettle();

    expect(find.text('暂无审计记录'), findsOneWidget);
  });
}
