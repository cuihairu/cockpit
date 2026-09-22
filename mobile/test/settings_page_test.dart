import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

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

class _FakeAuth extends AuthNotifier {
  _FakeAuth(this._initial);

  final AuthState _initial;

  @override
  AuthState build() => _initial;
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
}
