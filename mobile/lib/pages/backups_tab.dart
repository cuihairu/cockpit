import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// 备份 tab（资源页第三 tab）：任务列表 + 最近运行 + 手动触发。
class BackupsTab extends ConsumerStatefulWidget {
  const BackupsTab({super.key});

  @override
  ConsumerState<BackupsTab> createState() => _BackupsTabState();
}

class _BackupsTabState extends ConsumerState<BackupsTab>
    with AutomaticKeepAliveClientMixin {
  List<BackupConfig>? _configs;
  List<BackupRun>? _runs;
  String? _error;
  bool _loading = false;
  int? _running;

  @override
  bool get wantKeepAlive => true;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  Future<void> _reload() async {
    if (_loading) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final api = await ref.read(apiProvider.future);
      final results =
          await Future.wait([api.backupConfigs(), api.backupRuns(limit: 5)]);
      if (!mounted) return;
      setState(() {
        _configs = results[0] as List<BackupConfig>;
        _runs = results[1] as List<BackupRun>;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _trigger(BackupConfig c) async {
    setState(() => _running = c.id);
    try {
      final api = await ref.read(apiProvider.future);
      final status = await api.runBackup(c.id);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('${c.name} 已触发（$status）')));
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('${c.name} 触发失败：$e')));
    } finally {
      if (mounted) {
        setState(() => _running = null);
        _reload();
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    super.build(context);
    if (_error != null && _configs == null) {
      return ListView(children: [
        const SizedBox(height: 80),
        Center(child: Text('加载失败：$_error')),
      ]);
    }
    if (_configs == null) {
      return const Center(child: CircularProgressIndicator());
    }
    final configs = _configs!;
    final runs = _runs ?? const <BackupRun>[];
    return RefreshIndicator(
      onRefresh: _reload,
      child: ListView(
        children: [
          if (runs.isNotEmpty) ...[
            const Padding(
              padding: EdgeInsets.fromLTRB(16, 12, 16, 4),
              child: Text('最近运行',
                  style: TextStyle(fontWeight: FontWeight.bold)),
            ),
            for (final r in runs)
              ListTile(
                dense: true,
                leading: Icon(Icons.history,
                    color: _statusColor(r.status)),
                title: Text('任务 #${r.configId} · ${_fmtTime(r.startedAt)}'),
                subtitle: Text(
                    '${_fmtSize(r.size)}'
                    '${r.error.isEmpty ? '' : ' · ${r.error}'}'
                    '${r.remoteStatus == 'failed' ? ' · 异地推送失败' : r.remoteStatus == 'ok' ? ' · 异地已推送' : ''}',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis),
                trailing: _StatusChip(status: r.status),
              ),
            const Divider(),
          ],
          const Padding(
            padding: EdgeInsets.fromLTRB(16, 8, 16, 4),
            child: Text('备份任务',
                style: TextStyle(fontWeight: FontWeight.bold)),
          ),
          if (configs.isEmpty)
            const Padding(
              padding: EdgeInsets.all(32),
              child: Center(child: Text('暂无备份任务')),
            )
          else
            for (final c in configs)
              ListTile(
                leading: Icon(
                  c.enabled ? Icons.backup : Icons.backup_outlined,
                  color: c.enabled ? _statusColor(c.lastStatus) : Colors.grey,
                ),
                title: Text(c.name),
                subtitle: Text(
                    '${c.agentId.isEmpty ? '—' : c.agentId} · ${c.schedule}\n'
                    '${_lastRunText(c)}'),
                isThreeLine: true,
                trailing: _running == c.id
                    ? const SizedBox(
                        width: 18,
                        height: 18,
                        child:
                            CircularProgressIndicator(strokeWidth: 2))
                    : IconButton(
                        icon: const Icon(Icons.play_arrow),
                        tooltip: '立即运行',
                        onPressed: () => _trigger(c),
                      ),
              ),
        ],
      ),
    );
  }

  String _lastRunText(BackupConfig c) {
    if (c.lastStatus.isEmpty) return '尚未运行';
    final t = c.lastRunAt > 0 ? _fmtTime(c.lastRunAt) : '—';
    final next = c.schedule != 'manual' && c.nextRunAt > 0
        ? ' · 下次 ${_fmtTime(c.nextRunAt)}'
        : '';
    return '${_statusLabel(c.lastStatus)} $t$next';
  }

  Color _statusColor(String s) => switch (s) {
        'success' || 'ok' => Colors.green,
        'failed' || 'timeout' => Colors.red,
        'running' => Colors.blue,
        _ => Colors.grey,
      };

  String _statusLabel(String s) => switch (s) {
        'success' => '成功',
        'failed' => '失败',
        'timeout' => '超时',
        'running' => '运行中',
        _ => s.isEmpty ? '未运行' : s,
      };

  String _fmtTime(int unixSeconds) {
    if (unixSeconds <= 0) return '—';
    final t = DateTime.fromMillisecondsSinceEpoch(unixSeconds * 1000);
    final mm = t.month.toString().padLeft(2, '0');
    final dd = t.day.toString().padLeft(2, '0');
    final hh = t.hour.toString().padLeft(2, '0');
    final mi = t.minute.toString().padLeft(2, '0');
    return '$mm-$dd $hh:$mi';
  }

  String _fmtSize(int bytes) {
    if (bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    var v = bytes.toDouble();
    var u = 0;
    while (v >= 1024 && u < units.length - 1) {
      v /= 1024;
      u++;
    }
    return '${v.toStringAsFixed(v >= 10 || u == 0 ? 0 : 1)} ${units[u]}';
  }
}

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    final color = switch (status) {
      'success' => Colors.green,
      'failed' || 'timeout' => Colors.red,
      'running' => Colors.blue,
      _ => Colors.grey,
    };
    final label = switch (status) {
      'success' => '成功',
      'failed' => '失败',
      'timeout' => '超时',
      'running' => '运行中',
      _ => status,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(label, style: TextStyle(color: color, fontSize: 12)),
    );
  }
}
