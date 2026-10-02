import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../api/client.dart';
import '../api/endpoints.dart';
import '../theme/app_theme.dart';
import 'auth.dart';

/// 非敏感配置走 SharedPreferences，token 走 secure storage（D4）。
class SettingsState {
  const SettingsState({
    this.serverUrl = '',
    this.allowSelfSigned = false,
    this.biometricLock = false,
    this.themeMode = 'system',
    this.themeSkin = defaultAppSkin,
    this.loaded = false,
  });

  final String serverUrl;
  final bool allowSelfSigned;
  final bool biometricLock;

  /// 明暗档位：system / light / dark（与 Web 端 UITheme 同名同义）
  final String themeMode;

  /// 皮肤 key（与 Web 端 THEME_SKINS 同名同值，见 theme/app_theme.dart）
  final String themeSkin;

  final bool loaded;

  bool get configured => serverUrl.isNotEmpty;

  SettingsState copyWith({
    String? serverUrl,
    bool? allowSelfSigned,
    bool? biometricLock,
    String? themeMode,
    String? themeSkin,
    bool? loaded,
  }) =>
      SettingsState(
        serverUrl: serverUrl ?? this.serverUrl,
        allowSelfSigned: allowSelfSigned ?? this.allowSelfSigned,
        biometricLock: biometricLock ?? this.biometricLock,
        themeMode: themeMode ?? this.themeMode,
        themeSkin: themeSkin ?? this.themeSkin,
        loaded: loaded ?? this.loaded,
      );
}

class SettingsNotifier extends Notifier<SettingsState> {
  static const _serverKey = 'server_url';
  static const _selfSignedKey = 'allow_self_signed';
  static const _biometricKey = 'biometric_lock';
  static const _themeModeKey = 'theme_mode';
  static const _themeSkinKey = 'theme_skin';

  final _secure = const FlutterSecureStorage();
  SharedPreferences? _prefs;

  @override
  SettingsState build() {
    _load();
    return const SettingsState();
  }

  Future<void> _load() async {
    _prefs = await SharedPreferences.getInstance();
    final url = _prefs!.getString(_serverKey) ?? '';
    final selfSigned = _prefs!.getBool(_selfSignedKey) ?? false;
    final biometric = _prefs!.getBool(_biometricKey) ?? false;
    // 存量 app 无外观键：缺省跟随系统 + 默认皮肤
    final mode = _prefs!.getString(_themeModeKey) ?? 'system';
    final skin = _prefs!.getString(_themeSkinKey) ?? defaultAppSkin;
    state = SettingsState(
        serverUrl: url,
        allowSelfSigned: selfSigned,
        biometricLock: biometric,
        themeMode: mode,
        themeSkin: skin,
        loaded: true);
  }

  Future<void> setServerUrl(String url) async {
    _prefs ??= await SharedPreferences.getInstance();
    await _prefs!.setString(_serverKey, url);
    state = state.copyWith(serverUrl: url);
  }

  Future<void> setAllowSelfSigned(bool v) async {
    _prefs ??= await SharedPreferences.getInstance();
    await _prefs!.setBool(_selfSignedKey, v);
    state = state.copyWith(allowSelfSigned: v);
  }

  Future<void> setBiometricLock(bool v) async {
    _prefs ??= await SharedPreferences.getInstance();
    await _prefs!.setBool(_biometricKey, v);
    state = state.copyWith(biometricLock: v);
  }

  /// 切外观即时生效：落盘 + 广播 state，MaterialApp 随之重算主题
  Future<void> setThemeMode(String mode) async {
    _prefs ??= await SharedPreferences.getInstance();
    await _prefs!.setString(_themeModeKey, mode);
    state = state.copyWith(themeMode: mode);
  }

  Future<void> setThemeSkin(String skin) async {
    _prefs ??= await SharedPreferences.getInstance();
    await _prefs!.setString(_themeSkinKey, skin);
    state = state.copyWith(themeSkin: skin);
  }

  Future<String?> readToken() =>
      _secure.read(key: 'auth_token');

  Future<void> saveToken(String token) =>
      _secure.write(key: 'auth_token', value: token);

  Future<void> clearToken() => _secure.delete(key: 'auth_token');
}

final settingsProvider =
    NotifierProvider<SettingsNotifier, SettingsState>(SettingsNotifier.new);

/// 依赖 settings 重建：server URL / 自签开关变化时整个 API 栈重建。
/// 未配置 server 时读它抛 ServerNotConfigured，由引导流程捕获。
class ServerNotConfigured implements Exception {
  const ServerNotConfigured();
}

final apiProvider = FutureProvider<CockpitApi>((ref) async {
  final s = ref.watch(settingsProvider);
  if (!s.configured) throw const ServerNotConfigured();
  final settings = ref.read(settingsProvider.notifier);
  final token = await settings.readToken();
  final client = await ApiClient.create(
    baseUrl: s.serverUrl,
    allowSelfSigned: s.allowSelfSigned,
    initialToken: token,
    onTokenRefreshed: (t) => settings.saveToken(t),
    onUnauthorized: () async {
      await settings.clearToken();
      ref.read(authProvider.notifier).signedOut();
    },
  );
  return CockpitApi(client);
});
