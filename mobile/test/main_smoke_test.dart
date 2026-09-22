import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:cockpit_mobile/main.dart' as app;

// main() 全链路会 runApp 挂整棵树，且真请求依赖 flutter_test 的
// HttpOverrides 劫持快速失败——与任何其他 pumpWidget 测试同文件会互相
// 污染（遗留 Ticker 在重置后的时钟上触发 elapsedInSeconds >= 0 断言），
// 因此 runApp 冒烟用例独占一个文件。

const _storageChannel =
    MethodChannel('plugins.it_nomads.com/flutter_secure_storage');
const _authChannel = MethodChannel('plugins.flutter.io/local_auth');

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

void _mockLocalAuth() {
  TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
      .setMockMethodCallHandler(_authChannel, (call) async {
    switch (call.method) {
      case 'isDeviceSupported':
        return true;
      case 'getAvailableBiometrics':
        return <String>['fingerprint'];
      case 'authenticate':
        return true;
      default:
        return null;
    }
  });
}

void main() {
  tearDown(() {
    TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_storageChannel, null);
    TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_authChannel, null);
  });

  testWidgets('main()：真链路启动 → token 恢复登录 → 直接进主框架',
      (tester) async {
    SharedPreferences.setMockInitialValues(
        {'server_url': 'http://127.0.0.1:9', 'biometric_lock': false});
    _mockSecureStorage({'auth_token': 'jwt-9'});
    _mockLocalAuth();

    app.main();
    // settings 异步加载 + bootstrap；dashboard 的真请求会被 flutter_test 的
    // HttpOverrides 劫持成 400 快速失败，settle 可收敛（否则零时长 FakeTimer 挂尾）
    await tester.pump();
    await tester.pumpAndSettle(const Duration(milliseconds: 50));

    expect(find.byType(NavigationBar), findsOneWidget);
    expect(find.text('仪表盘'), findsOneWidget);
  });
}
