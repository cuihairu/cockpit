import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// SMART 磁盘健康页（M3 路线图）：per-agent 只读快照。
/// 异常盘置顶与字段口径对齐 web Disk 页（failed > unknown > passed，
/// 同级按扇区数倒序）；巡检配置与告警留桌面端——「手机看、桌面改」。
class SmartPage extends ConsumerStatefulWidget {
  const SmartPage({super.key, required this.agent});

  final Agent agent;

  @override
  ConsumerState<SmartPage> createState() => _SmartPageState();
}

class _SmartPageState extends ConsumerState<SmartPage> {
  late Future<SmartStatus> _future;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future = ref
        .read(apiProvider.future)
        .then((api) => api.smartStatus(widget.agent.id));
  }

  // 健康三态（对齐 web HEALTH_META：passed 绿 / failed 红 / unknown 灰）
  static const _healthMeta = <String, (String, Color, IconData)>{
    'passed': ('健康', Colors.green, Icons.check_circle),
    'failed': ('FAILED', Colors.red, Icons.error),
    'unknown': ('未知', Colors.grey, Icons.help_outline),
  };

  (String, Color, IconData) _meta(String health) =>
      _healthMeta[health] ?? _healthMeta['unknown']!;

  int _severity(SmartDevice d) =>
      d.health == 'failed' ? 0 : d.health == 'unknown' ? 1 : 2;

  int _badSectors(SmartDevice d) =>
      (d.reallocatedSectors ?? 0) +
      (d.pendingSectors ?? 0) +
      (d.mediaErrors ?? 0);

  String _fmtSize(int? bytes) {
    if (bytes == null || bytes <= 0) return '—';
    final gb = bytes / 1024 / 1024 / 1024;
    return gb >= 1000
        ? '${(gb / 1000).toStringAsFixed(1)} TB'
        : '${gb.round()} GB';
  }

  String _orDash(int? v) => v == null ? '—' : '$v';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('磁盘健康 · ${widget.agent.hostname}')),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<SmartStatus>(
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
            if (!r.available || r.devices.isEmpty) {
              return ListView(children: const [
                SizedBox(height: 80),
                Center(child: Text('未发现可读 SMART 的磁盘（需 root 权限运行 Agent）')),
              ]);
            }
            // 异常盘置顶：failed > unknown > passed，同级按扇区数倒序
            final devices = [...r.devices]..sort((a, b) =>
                _severity(a) != _severity(b)
                    ? _severity(a) - _severity(b)
                    : _badSectors(b) - _badSectors(a));
            return ListView(children: [
              for (final d in devices)
                ListTile(
                  leading: Icon(_meta(d.health).$3, color: _meta(d.health).$2),
                  title: Text(d.name),
                  subtitle: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text([
                        if (d.model.isNotEmpty) '型号 ${d.model}',
                        '容量 ${_fmtSize(d.sizeBytes)}',
                        '温度 ${d.temperatureC == null ? '—' : '${d.temperatureC}°C'}',
                      ].join(' · ')),
                      Text(
                          '重映射 ${_orDash(d.reallocatedSectors)} · 待定扇区 ${_orDash(d.pendingSectors)} · 介质错误 ${_orDash(d.mediaErrors)} · 通电 ${_orDash(d.powerOnHours)}h'
                          '${d.percentUsed != null ? ' · 使用率 ${d.percentUsed}%' : ''}'),
                      if (d.health == 'unknown' && d.error != null)
                        Text(d.error!,
                            style: TextStyle(
                                fontSize: 12,
                                color: Theme.of(context)
                                    .colorScheme
                                    .onSurfaceVariant)),
                    ],
                  ),
                  isThreeLine: d.health == 'unknown' && d.error != null,
                  trailing: Text(_meta(d.health).$1,
                      style: TextStyle(
                          color: _meta(d.health).$2,
                          fontWeight: FontWeight.w600)),
                ),
            ]);
          },
        ),
      ),
    );
  }
}
