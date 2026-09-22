import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'settings.dart';

sealed class AuthState {
  const AuthState();
}

/// 未配置 server 地址。
class AuthUnconfigured extends AuthState {
  const AuthUnconfigured();
}

/// 未登录。
class AuthAnonymous extends AuthState {
  const AuthAnonymous();
}

/// 密码通过，等待 TOTP 验证码。
class AuthTotpRequired extends AuthState {
  const AuthTotpRequired(this.tmpToken);

  final String tmpToken;
}

/// 已登录（token 已持久化）。
class Authenticated extends AuthState {
  const Authenticated({required this.username, required this.role});

  final String username;
  final String role;
}

class AuthNotifier extends Notifier<AuthState> {
  @override
  AuthState build() => const AuthUnconfigured();

  /// 启动恢复：settings 就绪后调用；本地有 token 即视为已登录，
  /// 失效由 401 拦截器刷新、刷新失败走 signedOut。
  Future<void> bootstrap() async {
    final token = await ref.read(settingsProvider.notifier).readToken();
    final configured = ref.read(settingsProvider).configured;
    if (!configured) {
      state = const AuthUnconfigured();
    } else if (token != null && token.isNotEmpty) {
      state = const Authenticated(username: '', role: '');
    } else {
      state = const AuthAnonymous();
    }
  }

  Future<void> login(String username, String password) async {
    final api = await ref.read(apiProvider.future);
    final resp = await api.login(username, password);
    if (resp.requiresTotp) {
      state = AuthTotpRequired(resp.tmpToken ?? '');
      return;
    }
    await _complete(resp.token ?? '', resp.username, resp.role ?? '');
  }

  Future<void> verifyTotp(String code) async {
    final s = state;
    if (s is! AuthTotpRequired) return;
    final api = await ref.read(apiProvider.future);
    final resp = await api.verifyTotp(code, s.tmpToken);
    await _complete(resp.token, resp.username, resp.role);
  }

  Future<void> _complete(String token, String username, String role) async {
    final settings = ref.read(settingsProvider.notifier);
    await settings.saveToken(token);
    // 刷新后的 token 同步进现有 Dio 拦截链，避免重建 API 栈。
    final api = await ref.read(apiProvider.future);
    api.client.updateToken(token);
    state = Authenticated(username: username, role: role);
  }

  void cancelTotp() => state = const AuthAnonymous();

  Future<void> logout() async {
    await ref.read(settingsProvider.notifier).clearToken();
    state = const AuthAnonymous();
  }

  /// 401 拦截器回调：token 彻底失效。
  void signedOut() => state = const AuthAnonymous();
}

final authProvider = NotifierProvider<AuthNotifier, AuthState>(AuthNotifier.new);
