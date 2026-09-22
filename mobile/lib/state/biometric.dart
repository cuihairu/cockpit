import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:local_auth/local_auth.dart';

/// 生物识别抽象：测试环境（无 platform channel）可替换。
abstract class BiometricAuth {
  Future<bool> isSupported();
  Future<bool> authenticate(String reason);
}

class LocalAuthBiometric implements BiometricAuth {
  final LocalAuthentication _la = LocalAuthentication();

  @override
  Future<bool> isSupported() async {
    // 两个条件都满足才算可用：设备有安全硬件/录入（android 门控指纹、Face ID）
    if (!await _la.isDeviceSupported()) return false;
    return _la.canCheckBiometrics;
  }

  @override
  Future<bool> authenticate(String reason) => _la.authenticate(
        localizedReason: reason,
        biometricOnly: true, // 不回落设备 PIN——锁屏语义就是生物识别
        persistAcrossBackgrounding: true, // 认证中切走再回来不中断流程
      );
}

/// 全局唯一实例；widget 测试 override 为假实现。
final biometricProvider = Provider<BiometricAuth>((ref) => LocalAuthBiometric());
