import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';
import 'backups_tab.dart';

/// 域名/证书到期视图（M2，只读）：两 tab，状态徽标用后端 status。
class ResourcesPage extends StatelessWidget {
  const ResourcesPage({super.key});

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 3,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('资源'),
          bottom: const TabBar(tabs: [
            Tab(text: '域名'),
            Tab(text: '证书'),
            Tab(text: '备份'),
          ]),
        ),
        body: const TabBarView(
            children: [_DomainsTab(), _CertsTab(), BackupsTab()]),
      ),
    );
  }
}

Color _statusColor(String status) => switch (status) {
      'valid' => Colors.green,
      'expiring' => Colors.orange,
      'expired' => Colors.red,
      _ => Colors.grey,
    };

String _statusLabel(String status) => switch (status) {
      'valid' => '正常',
      'expiring' => '临期',
      'expired' => '已过期',
      _ => '未知',
    };

String _shortDate(String iso) => iso.length >= 10 ? iso.substring(0, 10) : iso;

class _AsyncList<T> extends ConsumerWidget {
  const _AsyncList({required this.load, required this.buildRow});

  final Future<List<T>> Function(CockpitApi api) load;
  final Widget Function(BuildContext context, T item) buildRow;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return RefreshIndicator(
      onRefresh: () async => ref.invalidate(apiProvider),
      child: FutureBuilder<List<T>>(
        future:
            ref.read(apiProvider.future).then((api) => load(api)),
        builder: (context, snap) {
          if (snap.connectionState != ConnectionState.done) {
            return const Center(child: CircularProgressIndicator());
          }
          if (snap.hasError) {
            return ListView(children: [
              const SizedBox(height: 80),
              Center(child: Text('加载失败：${snap.error}')),
            ]);
          }
          final list = snap.data ?? const <Never>[];
          if (list.isEmpty) {
            return ListView(children: const [
              SizedBox(height: 80),
              Center(child: Text('暂无数据')),
            ]);
          }
          return ListView.separated(
            itemCount: list.length,
            separatorBuilder: (_, _) => const Divider(height: 1),
            itemBuilder: (context, i) => buildRow(context, list[i]),
          );
        },
      ),
    );
  }
}

class _StatusBadge extends StatelessWidget {
  const _StatusBadge({required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    final color = _statusColor(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        _statusLabel(status),
        style: TextStyle(color: color, fontSize: 12),
      ),
    );
  }
}

class _DomainsTab extends StatelessWidget {
  const _DomainsTab();

  @override
  Widget build(BuildContext context) {
    return _AsyncList<Domain>(
      load: (api) => api.domains(),
      buildRow: (context, d) => ListTile(
        leading: Icon(Icons.language, color: _statusColor(d.status)),
        title: Text(d.name),
        subtitle: Text(
            '${d.registrar.isEmpty ? '—' : d.registrar} · ${d.dnsProvider.isEmpty ? '—' : d.dnsProvider}\n到期 ${_shortDate(d.expiryDate)}${d.autoRenew ? ' · 自动续费' : ''}'),
        isThreeLine: true,
        trailing: _StatusBadge(status: d.status),
      ),
    );
  }
}

class _CertsTab extends StatelessWidget {
  const _CertsTab();

  @override
  Widget build(BuildContext context) {
    return _AsyncList<Certificate>(
      load: (api) => api.certificates(),
      buildRow: (context, c) {
        final extra = [
          if (c.acmeProvider != null && c.acmeProvider!.isNotEmpty)
            c.acmeProvider!,
          c.type,
        ].where((e) => e.isNotEmpty).join(' · ');
        return ListTile(
          leading: Icon(Icons.verified_user, color: _statusColor(c.status)),
          title: Text(c.commonName),
          subtitle: Text(
              '$extra\n到期 ${_shortDate(c.expiryDate)}'
              '${c.daysRemaining != null ? '（剩 ${c.daysRemaining} 天）' : ''}'
              '${c.autoRenew ? ' · 自动续期' : ''}'),
          isThreeLine: true,
          trailing: _StatusBadge(status: c.status),
        );
      },
    );
  }
}
