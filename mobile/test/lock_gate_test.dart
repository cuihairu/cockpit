import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/state/biometric.dart';
import 'package:cockpit_mobile/widgets/lock_gate.dart';

class _FakeBiometric implements BiometricAuth {
  _FakeBiometric({this.grant = true, this.throwOnCall = false});

  bool grant;
  final bool throwOnCall;
  int calls = 0;

  @override
  Future<bool> isSupported() async => true;

  @override
  Future<bool> authenticate(String reason) async {
    calls++;
    if (throwOnCall) throw StateError('no channel');
    return grant;
  }
}

Future<void> _pump(WidgetTester tester, BiometricAuth bio) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [biometricProvider.overrideWithValue(bio)],
    child: const MaterialApp(
      home: LockGate(child: Text('SECRET-CONTENT')),
    ),
  ));
}

void main() {
  testWidgets('认证通过 → 露出受保护内容', (tester) async {
    final bio = _FakeBiometric(grant: true);
    await _pump(tester, bio);
    await tester.pumpAndSettle();

    expect(bio.calls, 1);
    expect(find.text('SECRET-CONTENT'), findsOneWidget);
    expect(find.text('已锁定'), findsNothing);
  });

  testWidgets('认证失败 → 停在蒙层，可重试', (tester) async {
    final bio = _FakeBiometric(grant: false);
    await _pump(tester, bio);
    await tester.pumpAndSettle();

    expect(find.text('SECRET-CONTENT'), findsNothing);
    expect(find.text('已锁定'), findsOneWidget);
    expect(find.text('验证未通过'), findsOneWidget);

    // 重试且通过 → 解锁
    bio.grant = true;
    await tester.tap(find.text('解锁'));
    await tester.pumpAndSettle();
    expect(bio.calls, 2);
    expect(find.text('SECRET-CONTENT'), findsOneWidget);
  });

  testWidgets('平台通道异常 → 保持锁定并提示', (tester) async {
    final bio = _FakeBiometric(throwOnCall: true);
    await _pump(tester, bio);
    await tester.pumpAndSettle();

    expect(find.text('SECRET-CONTENT'), findsNothing);
    expect(find.textContaining('无法调用生物识别'), findsOneWidget);
  });
}
