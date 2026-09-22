import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/state/biometric.dart';

const _channel = MethodChannel('plugins.flutter.io/local_auth');

void _mockLocalAuth({
  bool deviceSupported = true,
  bool canCheck = true,
  Object? authenticateResult,
}) {
  TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
      .setMockMethodCallHandler(_channel, (call) async {
    switch (call.method) {
      case 'isDeviceSupported':
        return deviceSupported;
      case 'getAvailableBiometrics':
        return canCheck ? <String>['fingerprint'] : <String>[];
      case 'authenticate':
        if (authenticateResult is Exception) throw authenticateResult;
        return authenticateResult as bool? ?? true;
      default:
        return null;
    }
  });
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  tearDown(() {
    TestWidgetsFlutterBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(_channel, null);
  });

  test('isSupported：设备支持且可查 → true', () async {
    _mockLocalAuth();
    expect(await LocalAuthBiometric().isSupported(), isTrue);
  });

  test('isSupported：设备无安全硬件 → false', () async {
    _mockLocalAuth(deviceSupported: false);
    expect(await LocalAuthBiometric().isSupported(), isFalse);
  });

  test('isSupported：无录入 → false', () async {
    _mockLocalAuth(canCheck: false);
    expect(await LocalAuthBiometric().isSupported(), isFalse);
  });

  test('authenticate：通道确认 → true / 拒绝 → false', () async {
    _mockLocalAuth();
    expect(await LocalAuthBiometric().authenticate('解锁'), isTrue);

    _mockLocalAuth(authenticateResult: false);
    expect(await LocalAuthBiometric().authenticate('解锁'), isFalse);
  });
}
