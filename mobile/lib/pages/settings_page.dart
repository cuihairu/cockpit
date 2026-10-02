import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/auth.dart';
import '../state/biometric.dart';
import '../state/settings.dart';
import '../theme/app_theme.dart';
import 'audit_page.dart';

/// 设置：server 信息、主题外观、自签开关、退出登录。
class SettingsPage extends ConsumerWidget {
  const SettingsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(settingsProvider);
    final auth = ref.watch(authProvider);
    final username =
        auth is Authenticated && auth.username.isNotEmpty ? auth.username : '已登录';
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        ListTile(
          leading: const Icon(Icons.dns),
          title: const Text('Server'),
          subtitle: Text(s.serverUrl.isEmpty ? '未配置' : s.serverUrl),
          trailing: const Icon(Icons.chevron_right),
          onTap: () => _showChangeServer(context),
        ),
        ListTile(
          leading: const Icon(Icons.palette_outlined),
          title: const Text('主题外观'),
          subtitle: Text(
            '${themeModeLabels[s.themeMode] ?? '跟随系统'} · '
            '${resolveAppSkin(s.themeSkin).name}',
          ),
          trailing: const Icon(Icons.chevron_right),
          onTap: () => _showAppearance(context),
        ),
        SwitchListTile(
          secondary: const Icon(Icons.security),
          title: const Text('允许自签证书'),
          subtitle: const Text('存在中间人风险，仅在可信内网开启'),
          value: s.allowSelfSigned,
          onChanged: (v) async {
            await ref.read(settingsProvider.notifier).setAllowSelfSigned(v);
            ref.invalidate(apiProvider);
          },
        ),
        _BiometricTile(enabled: s.biometricLock),
        const Divider(),
        ListTile(
          leading: const Icon(Icons.history),
          title: const Text('审计日志'),
          subtitle: const Text('操作留痕查询'),
          trailing: const Icon(Icons.chevron_right),
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute(builder: (_) => const AuditPage()),
          ),
        ),
        ListTile(
          leading: const Icon(Icons.logout),
          title: const Text('退出登录'),
          subtitle: Text(username),
          onTap: () => ref.read(authProvider.notifier).logout(),
        ),
      ],
    );
  }

  void _showChangeServer(BuildContext context) {
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text('修改地址请退出登录后在引导页重新配置（M1）')));
  }

  void _showAppearance(BuildContext context) {
    showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      builder: (_) => const _AppearanceSheet(),
    );
  }
}

/// 主题外观面板：明暗档位 + 皮肤（每项带该皮肤自身色板绘制的缩略预览，
/// 与 Web 端设置页「主题皮肤」卡片同思路：所见即切换后的效果）。
class _AppearanceSheet extends ConsumerWidget {
  const _AppearanceSheet();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(settingsProvider);
    final notifier = ref.read(settingsProvider.notifier);
    return SafeArea(
      child: ListView(
        shrinkWrap: true,
        children: [
          const Padding(
            padding: EdgeInsets.fromLTRB(16, 0, 16, 4),
            child: Text('明暗模式'),
          ),
          RadioGroup<String>(
            groupValue: s.themeMode,
            onChanged: (v) {
              if (v != null) notifier.setThemeMode(v);
            },
            child: Column(
              children: [
                for (final entry in themeModeLabels.entries)
                  RadioListTile<String>(
                    value: entry.key,
                    title: Text(entry.value),
                  ),
              ],
            ),
          ),
          const Divider(),
          const Padding(
            padding: EdgeInsets.fromLTRB(16, 8, 16, 4),
            child: Text('主题皮肤'),
          ),
          RadioGroup<String>(
            groupValue: s.themeSkin,
            onChanged: (v) {
              if (v != null) notifier.setThemeSkin(v);
            },
            child: Column(
              children: [
                for (final skin in appSkins)
                  RadioListTile<String>(
                    value: skin.key,
                    title: Text(skin.name),
                    subtitle: Text(skin.description),
                    secondary: _SkinPreview(skin: skin, dark: themeModeOf(s.themeMode) == ThemeMode.dark),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// 皮肤缩略图：用皮肤自身色板现画小窗（不依赖当前生效主题）。
class _SkinPreview extends StatelessWidget {
  const _SkinPreview({required this.skin, required this.dark});

  final AppSkin skin;
  final bool dark;

  @override
  Widget build(BuildContext context) {
    final p = skin.paletteFor(dark ? Brightness.dark : Brightness.light);
    return Container(
      width: 44,
      height: 34,
      decoration: BoxDecoration(
        color: p.bg,
        border: Border.all(color: p.border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Column(
        children: [
          Container(height: 8, color: p.surface),
          Expanded(
            child: Padding(
              padding: const EdgeInsets.all(4),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                      width: 8,
                      height: 14,
                      decoration: BoxDecoration(
                        color: p.surface,
                        border: Border(left: BorderSide(color: p.borderSoft)),
                      )),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Container(
                          width: 18,
                          height: 4,
                          color: p.text,
                        ),
                        const SizedBox(height: 2),
                        Container(
                          width: 11,
                          height: 3,
                          color: p.textMuted,
                        ),
                        const SizedBox(height: 2),
                        Container(
                          width: 26,
                          height: 4,
                          decoration: BoxDecoration(
                            color: p.surfaceAlt,
                            border: Border.all(color: p.borderSoft),
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// 生物识别锁开关：设备不支持时禁用；平台通道异常视为不支持。
class _BiometricTile extends ConsumerWidget {
  const _BiometricTile({required this.enabled});

  final bool enabled;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return FutureBuilder<bool>(
      future: ref.read(biometricProvider).isSupported(),
      builder: (context, snap) {
        final supported = snap.data ?? false;
        return SwitchListTile(
          secondary: const Icon(Icons.fingerprint),
          title: const Text('启动生物识别锁'),
          subtitle: Text(supported
              ? '打开应用需验证指纹或面容'
              : snap.hasError
                  ? '无法检测生物识别可用性'
                  : '本设备不支持生物识别'),
          value: enabled,
          onChanged: supported
              ? (v) =>
                  ref.read(settingsProvider.notifier).setBiometricLock(v)
              : null,
        );
      },
    );
  }
}
