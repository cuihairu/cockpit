import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/biometric.dart';

/// 生物识别锁（M2）：biometricLock 开启时盖在已登录 UI 上的启动门。
/// 认证通过才露出 child；失败/异常停留在蒙层，可重试。
class LockGate extends ConsumerStatefulWidget {
  const LockGate({super.key, required this.child});

  final Widget child;

  @override
  ConsumerState<LockGate> createState() => _LockGateState();
}

class _LockGateState extends ConsumerState<LockGate> {
  bool _locked = true;
  bool _checking = false;
  String _hint = '';

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _verify());
  }

  Future<void> _verify() async {
    if (_checking) return;
    setState(() {
      _checking = true;
      _hint = '';
    });
    try {
      final ok = await ref
          .read(biometricProvider)
          .authenticate('验证指纹或面容以解锁 Cockpit');
      if (!mounted) return;
      setState(() => _locked = !ok);
      if (!ok) setState(() => _hint = '验证未通过');
    } catch (e) {
      if (!mounted) return;
      setState(() => _hint = '无法调用生物识别：$e');
    } finally {
      if (mounted) setState(() => _checking = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!_locked) return widget.child;
    return Scaffold(
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.fingerprint,
                size: 72,
                color: Theme.of(context).colorScheme.primary),
            const SizedBox(height: 12),
            const Text('已锁定',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold)),
            const SizedBox(height: 4),
            Text(_hint.isEmpty ? '验证以继续使用' : _hint,
                style: TextStyle(color: Colors.grey.shade600, fontSize: 13)),
            const SizedBox(height: 16),
            FilledButton.icon(
              onPressed: _checking ? null : _verify,
              icon: _checking
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.fingerprint),
              label: const Text('解锁'),
            ),
          ],
        ),
      ),
    );
  }
}
