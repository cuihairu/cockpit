import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/api/client.dart';
import 'package:cockpit_mobile/api/endpoints.dart';
import 'package:cockpit_mobile/main.dart';
import 'package:cockpit_mobile/state/auth.dart';
import 'package:cockpit_mobile/state/settings.dart';

void main() {
  testWidgets('未配置 server → 引导页（连接测试）', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() =>
            _FakeSettings(serverUrl: '')),
        authProvider.overrideWith(() => _FakeAuth(const AuthUnconfigured())),
      ],
      child: const CockpitApp(),
    ));
    await tester.pump();
    expect(find.text('连接 Cockpit Server'), findsOneWidget);
    expect(find.text('测试并保存'), findsOneWidget);
    expect(find.text('允许自签证书'), findsOneWidget);
  });

  testWidgets('已配置 + TOTP 待验证 → 两步验证页', (tester) async {
    final dio = Dio(BaseOptions(baseUrl: 'http://test'));
    final api = CockpitApi(ApiClient.forTest(dio));
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        authProvider.overrideWith(() => _FakeAuth(AuthTotpRequired('tmp-1'))),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const CockpitApp(),
    ));
    await tester.pumpAndSettle();
    expect(find.text('两步验证'), findsOneWidget);
    expect(find.text('验证码'), findsOneWidget);
    expect(find.text('验证'), findsOneWidget);
    expect(find.text('登录'), findsNothing);
  });

  testWidgets('已配置 + 未登录 → 登录页', (tester) async {
    final dio = Dio(BaseOptions(baseUrl: 'http://test'));
    final api = CockpitApi(ApiClient.forTest(dio));
    await tester.pumpWidget(ProviderScope(
      overrides: [
        settingsProvider.overrideWith(() => _FakeSettings()),
        authProvider.overrideWith(() => _FakeAuth(const AuthAnonymous())),
        apiProvider.overrideWith((ref) async => api),
      ],
      child: const CockpitApp(),
    ));
    await tester.pumpAndSettle();
    expect(find.text('登录'), findsWidgets);
    expect(find.text('用户名'), findsOneWidget);
    expect(find.text('密码'), findsOneWidget);
  });
}

class _FakeSettings extends SettingsNotifier {
  _FakeSettings({this.serverUrl = 'http://test'});

  final String serverUrl;

  @override
  SettingsState build() =>
      SettingsState(serverUrl: serverUrl, loaded: true);

  @override
  Future<String?> readToken() async => null;

  @override
  Future<void> saveToken(String token) async {}

  @override
  Future<void> clearToken() async {}
}

class _FakeAuth extends AuthNotifier {
  _FakeAuth(this._initial);

  final AuthState _initial;

  @override
  AuthState build() => _initial;
}
