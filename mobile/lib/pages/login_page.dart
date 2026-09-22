import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/auth.dart';

/// 登录页：密码认证 → requires_totp 时切验证码二步（对齐 web LoginResponse 双步流程）。
class LoginPage extends ConsumerStatefulWidget {
  const LoginPage({super.key});

  @override
  ConsumerState<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends ConsumerState<LoginPage> {
  final _username = TextEditingController();
  final _password = TextEditingController();
  final _code = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _username.dispose();
    _password.dispose();
    _code.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    final auth = ref.read(authProvider.notifier);
    try {
      final s = ref.read(authProvider);
      if (s is AuthTotpRequired) {
        await auth.verifyTotp(_code.text.trim());
      } else {
        await auth.login(_username.text.trim(), _password.text);
      }
    } on DioException catch (e) {
      final data = e.response?.data;
      String msg = e.message ?? e.type.name;
      if (data is Map && data['error'] != null) msg = data['error'].toString();
      setState(() => _error = msg);
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final auth = ref.watch(authProvider);
    final totp = auth is AuthTotpRequired;
    return Scaffold(
      appBar: AppBar(
        title: Text(totp ? '两步验证' : '登录'),
        actions: [
          if (totp)
            TextButton(
              onPressed: () =>
                  ref.read(authProvider.notifier).cancelTotp(),
              child: const Text('返回'),
            ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          if (!totp) ...[
            TextField(
              controller: _username,
              autofillHints: const [AutofillHints.username],
              decoration: const InputDecoration(
                  labelText: '用户名', border: OutlineInputBorder()),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _password,
              obscureText: true,
              autofillHints: const [AutofillHints.password],
              onSubmitted: (_) => _submit(),
              decoration: const InputDecoration(
                  labelText: '密码', border: OutlineInputBorder()),
            ),
          ] else ...[
            TextField(
              controller: _code,
              keyboardType: TextInputType.number,
              maxLength: 6,
              autofocus: true,
              onSubmitted: (_) => _submit(),
              decoration: const InputDecoration(
                  labelText: '验证码', border: OutlineInputBorder()),
            ),
          ],
          const SizedBox(height: 16),
          FilledButton(
            onPressed: _busy ? null : _submit,
            child: _busy
                ? const SizedBox(
                    width: 18, height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : Text(totp ? '验证' : '登录'),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!,
                style:
                    TextStyle(color: Theme.of(context).colorScheme.error)),
          ],
        ],
      ),
    );
  }
}
