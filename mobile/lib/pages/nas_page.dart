import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// NAS 存储观测页（M3 路线图）：per-agent 只读快照——存储池 / 挂载点容量 /
/// 网络共享三段，异常池置顶、挂载超阈值红高亮（阈值取全局巡检配置
/// usage_warn_percent，配置读取失败按 server 默认 80 呈现）。
/// 巡检配置与告警留桌面端——「手机看、桌面改」。
class NasPage extends ConsumerStatefulWidget {
  const NasPage({super.key, required this.agent});

  final Agent agent;

  @override
  ConsumerState<NasPage> createState() => _NasPageState();
}

/// 页面数据：快照 + 红高亮阈值（config 拉取失败时兜底 server 默认值）。
class NasPageData {
  final NasStatus status;
  final int warnPct;

  NasPageData({required this.status, required this.warnPct});
}

class _NasPageState extends ConsumerState<NasPage> {
  late Future<NasPageData> _future;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future = ref.read(apiProvider.future).then((api) async {
      final status = await api.nasStatus(widget.agent.id);
      var warnPct = 80; // server 默认阈值
      try {
        warnPct = (await api.nasConfig()).usageWarnPercent;
      } catch (_) {
        // 配置端点失败不阻塞观测（旧版本 server 无该端点等），红高亮按默认阈值
      }
      return NasPageData(status: status, warnPct: warnPct);
    });
  }

  // 池状态（对齐 web POOL_META：healthy 绿 / degraded、failed 红 / resync 蓝 / unknown 灰）
  static const _poolMeta = <String, (String, Color)>{
    'healthy': ('健康', Colors.green),
    'degraded': ('降级', Colors.red),
    'resync': ('同步中', Colors.blue),
    'failed': ('故障', Colors.red),
    'unknown': ('未知', Colors.grey),
  };

  static const _kindLabel = <String, String>{
    'mdadm': 'mdadm',
    'zfs': 'ZFS',
    'lvm': 'LVM',
    'dsm': 'DSM',
    'truenas': 'TrueNAS',
    'omv': 'OMV',
  };

  static const _shareProtocolLabel = <String, String>{
    'smb': 'SMB',
    'nfs': 'NFS',
  };

  // 池异常严重度：failed > degraded/resync > unknown > healthy（对齐 web）
  int _poolSeverity(String state) => switch (state) {
        'failed' => 0,
        'degraded' || 'resync' => 1,
        'unknown' => 2,
        _ => 3,
      };

  // web formatGB：GB→TB 按 1024 进位（与 smart 的字节换算不同）
  String _fmtGB(double gb) {
    if (gb <= 0) return '—';
    return gb >= 1024
        ? '${(gb / 1024).toStringAsFixed(1)} TB'
        : '${gb.round()} GB';
  }

  String _hostSuffix(String host) => host.isEmpty ? '' : ' · $host';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('存储观测 · ${widget.agent.hostname}')),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<NasPageData>(
          future: _future,
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
            final r = snap.data!;
            final s = r.status;
            if (!s.available) {
              return ListView(children: const [
                SizedBox(height: 80),
                Center(child: Text('未发现可观测的存储（mdadm/ZFS/LVM/SMB/NFS 均无数据）')),
              ]);
            }
            final pools = [...s.pools]..sort((a, b) =>
                _poolSeverity(a.state) != _poolSeverity(b.state)
                    ? _poolSeverity(a.state) - _poolSeverity(b.state)
                    : 0);
            final allEmpty =
                pools.isEmpty && s.mounts.isEmpty && s.shares.isEmpty;
            return ListView(children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
                child: Text('数据源：${s.source.isEmpty ? '—' : s.source}',
                    style:
                        const TextStyle(fontWeight: FontWeight.bold)),
              ),
              if (allEmpty)
                const Padding(
                  padding: EdgeInsets.all(32),
                  child: Center(child: Text('未发现存储池/挂载点/网络共享')),
                ),
              if (pools.isNotEmpty) ..._sectionTitle('存储池'),
              for (final p in pools)
                ListTile(
                  leading: Icon(
                    _poolMeta[p.state]?.$2 == Colors.red
                        ? Icons.warning_amber
                        : Icons.album,
                    color: _poolMeta[p.state]?.$2 ?? Colors.grey,
                  ),
                  title: Text('${p.name}${_hostSuffix(p.host)}'),
                  subtitle: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                          '${_kindLabel[p.kind] ?? p.kind} · 容量 ${_fmtGB(p.totalGB)} · 已用 ${_fmtGB(p.usedGB)}'),
                      Text(p.devices.isEmpty
                          ? '成员盘 —'
                          : '成员盘 ${p.devices.join('、')}'),
                      if (p.detail.isNotEmpty && p.state != 'healthy')
                        Text(p.detail,
                            style: TextStyle(
                                fontSize: 12,
                                color: Theme.of(context)
                                    .colorScheme
                                    .onSurfaceVariant)),
                    ],
                  ),
                  isThreeLine:
                      p.detail.isNotEmpty && p.state != 'healthy',
                  trailing: Text(_poolMeta[p.state]?.$1 ?? p.state,
                      style: TextStyle(
                          color: _poolMeta[p.state]?.$2 ?? Colors.grey,
                          fontWeight: FontWeight.w600)),
                ),
              if (s.mounts.isNotEmpty) ..._sectionTitle('挂载点容量'),
              for (final m in s.mounts)
                ListTile(
                  leading: const Icon(Icons.storage),
                  title: Text(m.mountPath, style: const TextStyle(fontFamily: 'monospace')),
                  subtitle: Text(
                      '${m.device} · ${m.fsType} · 容量 ${_fmtGB(m.totalGB)}${_hostSuffix(m.host)}'),
                  trailing: _usageText(m, r.warnPct),
                ),
              if (s.shares.isNotEmpty) ..._sectionTitle('网络共享'),
              for (final sh in s.shares)
                ListTile(
                  leading: const Icon(Icons.folder_shared),
                  title: Text(
                      '${_shareProtocolLabel[sh.protocol] ?? sh.protocol} · ${sh.name}'),
                  subtitle: Text(
                      '${sh.path}${sh.comment.isNotEmpty ? ' · 说明 ${sh.comment}' : ''}${sh.hosts.isNotEmpty ? ' · 允许 ${sh.hosts}' : ''}${_hostSuffix(sh.host)}'),
                ),
              const SizedBox(height: 16),
            ]);
          },
        ),
      ),
    );
  }

  List<Widget> _sectionTitle(String title) => [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
          child: Text(title, style: const TextStyle(fontWeight: FontWeight.bold)),
        ),
      ];

  /// 挂载已用百分比 + 用量；超阈值红高亮（对齐 web #cf1322 + w600）。
  Widget _usageText(NasMount m, int warnPct) {
    if (m.totalGB <= 0) return const Text('—');
    final pct = ((m.usedGB / m.totalGB) * 100).round();
    final hot = pct >= warnPct;
    return Text(
      '$pct%（${_fmtGB(m.usedGB)}）',
      style: hot
          ? const TextStyle(
              color: Color(0xFFCF1322), fontWeight: FontWeight.w600)
          : null,
    );
  }
}
