import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/auth.dart';
import '../state/settings.dart';
import 'audit_page.dart';

/// 设置：server 信息、自签开关、退出登录。
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
}
