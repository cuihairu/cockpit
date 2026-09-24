import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/state/biometric.dart';
import 'package:cockpit_mobile/widgets/lock_gate.dart';

/// 可脚本化的 BiometricAuth fake：result 返回 bool 或抛出；
/// gate 非空时 authenticate 先挂起，测试手动放行以制造时序。
class _FakeBiometric implements BiometricAuth {
  _FakeBiometric({this.result, this.gate});

  final Object? Function()? result;
  Completer<void>? gate;

  @override
  Future<bool> isSupported() async => true;

  @override
  Future<bool> authenticate(String reason) async {
    final g = gate;
    if (g != null) await g.future;
    final r = result?.call();
    if (r is Exception) throw r;
    return (r as bool?) ?? true;
  }
}

Future<void> _pump(WidgetTester tester, BiometricAuth bio,
    {Widget? home}) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [biometricProvider.overrideWithValue(bio)],
    child: MaterialApp(home: home ?? LockGate(child: const Text('受保护内容'))),
  ));
  // post-frame 回调触发 _verify
  await tester.pump();
}

void main() {
  testWidgets('LockGate：认证通过 → 露出受保护内容', (tester) async {
    await _pump(tester, _FakeBiometric(result: () => true));
    await tester.pumpAndSettle();
    expect(find.text('受保护内容'), findsOneWidget);
  });

  testWidgets('LockGate：认证未通过 → 蒙层提示验证未通过，内容不露出',
      (tester) async {
    await _pump(tester, _FakeBiometric(result: () => false));
    await tester.pumpAndSettle();

    expect(find.text('已锁定'), findsOneWidget);
    expect(find.text('验证未通过'), findsOneWidget);
    expect(find.text('受保护内容'), findsNothing);
  });

  testWidgets('LockGate：认证抛异常 → 提示无法调用生物识别', (tester) async {
    await _pump(
        tester,
        _FakeBiometric(
            result: () => Exception(StateError('no hardware'))));
    await tester.pumpAndSettle();

    expect(find.textContaining('无法调用生物识别'), findsOneWidget);
    expect(find.text('受保护内容'), findsNothing);
  });

  testWidgets('LockGate：认证挂起期间页面被销毁 → 静默放弃，不触发已卸载 setState',
      (tester) async {
    final gate = Completer<void>();
    final bio = _FakeBiometric(
        gate: gate, result: () => throw StateError('late failure'));
    await _pump(tester, bio);
    expect(find.text('已锁定'), findsOneWidget); // 认证中已在蒙层

    // 卸载整棵树后认证才失败：catch 分支的 mounted 检查应拦截
    await tester.pumpWidget(const SizedBox());
    gate.complete();
    await tester.pump();
  });

  testWidgets('LockGate：锁定蒙层被强制弹出（didPop）→ 正常移除', (tester) async {
    final bio = _FakeBiometric(result: () => false);
    await _pump(
        tester,
        bio,
        home: Builder(builder: (context) => TextButton(
              onPressed: () => Navigator.push(
                  context,
                  MaterialPageRoute<void>(
                      builder: (_) =>
                          LockGate(child: const Text('受保护内容')))),
              child: const Text('进入'),
            )));
    await tester.pumpAndSettle();

    // 进入二级路由（LockGate 锁定），再强制 pop：didPop=true 分支
    await tester.tap(find.text('进入'));
    await tester.pumpAndSettle();
    expect(find.text('已锁定'), findsOneWidget);

    final navigator =
        tester.state<NavigatorState>(find.byType(Navigator).first);
    navigator.pop();
    await tester.pumpAndSettle();

    expect(find.text('已锁定'), findsNothing);
    expect(find.text('进入'), findsOneWidget);
  });
}
