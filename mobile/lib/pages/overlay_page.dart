import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// 组网观测页（M3 路线图）：per-agent 只读快照——工具卡（状态/版本/
/// 网络/对端/WireGuard 接口）+ capability metadata 里的本机虚拟网身份。
/// join/leave、daemon 启停等写操作留桌面端——「手机看、桌面改」。
class OverlayPage extends ConsumerStatefulWidget {
  const OverlayPage({super.key, required this.agent});

  final Agent agent;

  @override
  ConsumerState<OverlayPage> createState() => _OverlayPageState();
}

class _OverlayPageState extends ConsumerState<OverlayPage> {
  late Future<OverlayStatus> _future;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future = ref
        .read(apiProvider.future)
        .then((api) => api.overlayStatus(widget.agent.id));
  }

  // 对齐 web Network 页 TOOL_LABELS / STATUS_LABELS / STATUS_COLORS
  static const _toolLabel = <String, String>{
    'zerotier': 'ZeroTier',
    'tailscale': 'Tailscale',
    'wireguard': 'WireGuard',
    'frp': 'frp',
  };

  static const _statusMeta = <String, (String, Color)>{
    'ok': ('正常', Colors.green),
    'degraded': ('降级', Colors.orange),
    'error': ('错误', Colors.red),
    'unavailable': ('未安装', Colors.grey),
  };

  String _peerName(OverlayPeer p) => p.name.isNotEmpty ? p.name : p.id;

  /// capability metadata.identity（detector extractIdentity 上报）：
  /// ZT {nodeId, networks[]} / TS {id, hostName}——缺席字段跳过。
  List<String> _identityChips() {
    Map<String, dynamic>? identity;
    for (final c in widget.agent.capabilities) {
      if (c.type == 'overlay') {
        identity = c.metadata?['identity'] as Map<String, dynamic>?;
        break;
      }
    }
    if (identity == null) return const [];
    final chips = <String>[];
    final zt = identity['zerotier'] as Map<String, dynamic>?;
    final nodeId = zt?['nodeId'] as String?;
    if (nodeId != null && nodeId.isNotEmpty) {
      final nets = (zt?['networks'] as List<dynamic>? ?? []).length;
      chips.add('ZeroTier $nodeId${nets > 0 ? '（$nets 个网络）' : ''}');
    }
    final ts = identity['tailscale'] as Map<String, dynamic>?;
    final tsName = (ts?['hostName'] as String?) ?? (ts?['id'] as String?);
    if (tsName != null && tsName.isNotEmpty) chips.add('Tailscale $tsName');
    return chips;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('组网观测 · ${widget.agent.hostname}')),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<OverlayStatus>(
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
            final tools = snap.data!.tools
                .where((t) => t.status != 'unavailable')
                .toList();
            final identity = _identityChips();
            if (tools.isEmpty) {
              return ListView(children: [
                if (identity.isNotEmpty) ..._identityCard(identity),
                const SizedBox(height: 80),
                const Center(child: Text('未检测到组网工具')),
              ]);
            }
            return ListView(children: [
              if (identity.isNotEmpty) ..._identityCard(identity),
              for (final t in tools) _toolCard(t),
              const SizedBox(height: 16),
            ]);
          },
        ),
      ),
    );
  }

  List<Widget> _identityCard(List<String> chips) {
    return [
      Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
        child: Wrap(
          spacing: 8,
          runSpacing: 4,
          children: [
            for (final c in chips)
              Chip(
                label: Text(c),
                visualDensity: VisualDensity.compact,
                backgroundColor: Theme.of(context)
                    .colorScheme
                    .surfaceContainerHighest,
              ),
          ],
        ),
      ),
    ];
  }

  Widget _toolCard(OverlayTool t) {
    final meta = _statusMeta[t.status] ?? _statusMeta['unavailable']!;
    final label = _toolLabel[t.tool] ?? t.tool;
    return Card(
      margin: const EdgeInsets.fromLTRB(12, 12, 12, 0),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              dense: true,
              title: Text(
                  '$label${t.version.isNotEmpty ? ' · v${t.version}' : ''}',
                  style: const TextStyle(fontWeight: FontWeight.w600)),
              trailing: Text(meta.$1,
                  style: TextStyle(
                      color: meta.$2, fontWeight: FontWeight.w600)),
            ),
            if (t.error.isNotEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Text(t.error,
                    style: TextStyle(
                        fontSize: 12,
                        color: Theme.of(context).colorScheme.error)),
              ),
            if (t.tool == 'frp') _frpProcesses(t),
            for (final n in t.networks)
              ListTile(
                dense: true,
                leading: const Icon(Icons.lan),
                title: Text(n.name.isNotEmpty ? '${n.name}（${n.id}）' : n.id),
                subtitle: Text(
                    '${n.status.isEmpty ? (n.online == true ? '在线' : '离线') : n.status}'
                    '${n.ips.isNotEmpty ? ' · ${n.ips.join('、')}' : ''}'),
              ),
            for (final i in t.interfaces)
              ListTile(
                dense: true,
                leading: const Icon(Icons.cable),
                title: Text(i.name),
                subtitle: Text(
                    '${i.peerCount} 个 peer${i.listenPort.isNotEmpty ? ' · 监听 ${i.listenPort}' : ''}'),
              ),
            for (final p in t.peers)
              ListTile(
                dense: true,
                leading: Icon(
                  p.online ? Icons.check_circle : Icons.cancel,
                  color: p.online ? Colors.green : Colors.grey,
                  size: 20,
                ),
                title: Text(_peerName(p)),
                subtitle: Text(
                    '${p.virtualIps.isNotEmpty ? p.virtualIps.join('、') : (p.endpoint.isNotEmpty ? p.endpoint : '—')}'
                    '${p.latencyMs != null ? ' · ${p.latencyMs}ms' : ''}'
                    '${p.relay.isNotEmpty ? ' · 中继 ${p.relay}' : ''}'),
              ),
          ],
        ),
      ),
    );
  }

  /// frp 分级观测：extra.frpc/frps 进程态（web 端同样未展开，移动端给一行
  /// 只读提示；隧道计数需 admin API 配置，呈现留桌面端）。
  Widget _frpProcesses(OverlayTool t) {
    bool? runningOf(String key) {
      final m = t.extra?[key] as Map<String, dynamic>?;
      return m?['running'] as bool?;
    }

    final frpc = runningOf('frpc');
    final frps = runningOf('frps');
    if (frpc == null && frps == null) return const SizedBox.shrink();
    String flag(bool? v) => v == true ? '运行中' : '未运行';
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Text(
          'frpc ${flag(frpc)} · frps ${flag(frps)}',
          style: TextStyle(
              fontSize: 12,
              color: Theme.of(context).colorScheme.onSurfaceVariant)),
    );
  }
}
