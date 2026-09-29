import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// 应用部署（Stacks）页（M3 路线图）：stack 列表 + up/down/restart/pull
/// 异步操作（taskId 2s 轮询）+ compose/.env 只读预览。
/// compose 编辑、stack 删除与部署历史留桌面端——移动端只看与启停，
/// 与「手机看、桌面改」定位一致。
class StacksPage extends ConsumerStatefulWidget {
  const StacksPage({super.key, required this.agent});

  final Agent agent;

  @override
  ConsumerState<StacksPage> createState() => _StacksPageState();
}

class _StacksPageState extends ConsumerState<StacksPage> {
  late Future<StacksResult> _future;
  Timer? _pollTimer;
  String? _acting; // 正在下发动作的 stack 名（行内小 spinner）

  @override
  void initState() {
    super.initState();
    _reload();
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }

  void _reload() {
    _future = ref
        .read(apiProvider.future)
        .then((api) => api.stacks(widget.agent.id));
  }

  Future<void> _action(StackSummary s, String action) async {
    if (action == 'down' && !await _confirmDown(s)) return;
    setState(() => _acting = s.name);
    try {
      final api = await ref.read(apiProvider.future);
      final taskId = await api.stackAction(widget.agent.id, s.name, action);
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('${s.name} $action 已提交')));
      if (taskId.isNotEmpty) _startPolling(api, taskId, s.name, action);
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('${s.name} $action 失败：$e')));
    } finally {
      if (mounted) setState(() => _acting = null);
    }
  }

  /// down 会下线该应用全部服务，二次确认。
  Future<bool> _confirmDown(StackSummary s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text('停止 ${s.name}？'),
        content: Text('将下线该应用的全部 ${s.total} 个服务，确认继续？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: const Text('停止'),
          ),
        ],
      ),
    );
    return ok == true;
  }

  /// 启动类动作拿到 taskId 后 2s 轮询终态；新任务会顶掉旧轮询。
  void _startPolling(CockpitApi api, String taskId, String name, String action) {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(
        const Duration(seconds: 2), (_) => _pollOnce(api, taskId, name, action));
  }

  Future<void> _pollOnce(
      CockpitApi api, String taskId, String name, String action) async {
    StackTask t;
    try {
      t = await api.stackTask(widget.agent.id, taskId);
    } catch (_) {
      // 单次轮询失败不终止（agent 抖动/重启常见），下个周期重试；
      // 页面退出时 dispose 取消定时器兜底。
      return;
    }
    if (!t.done || !mounted) return;
    _pollTimer?.cancel();
    _pollTimer = null;
    if (t.status == 'success') {
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('$name $action 完成')));
      setState(_reload);
    } else {
      _showTaskLog(name, action, t);
    }
  }

  void _showTaskLog(String name, String action, StackTask t) {
    showModalBottomSheet<void>(
      context: context,
      builder: (sheetContext) => SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text('$name $action 失败',
                  style: const TextStyle(fontWeight: FontWeight.bold)),
              const SizedBox(height: 8),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: Theme.of(context)
                      .colorScheme
                      .surfaceContainerHighest
                      .withValues(alpha: 0.5),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: SelectableText(
                  t.log.isEmpty ? '（无日志输出）' : t.log,
                  style: const TextStyle(
                      fontFamily: 'monospace', fontSize: 12),
                ),
              ),
              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    );
  }

  void _showCompose(String name) {
    showModalBottomSheet<void>(
      context: context,
      builder: (sheetContext) => SafeArea(
        child: FutureBuilder<StackCompose>(
          future: ref
              .read(apiProvider.future)
              .then((api) => api.stackCompose(widget.agent.id, name)),
          builder: (context, snap) {
            if (snap.connectionState != ConnectionState.done) {
              return const SizedBox(
                  height: 120,
                  child: Center(child: CircularProgressIndicator()));
            }
            if (snap.hasError) {
              return SizedBox(
                  height: 120,
                  child: Center(child: Text('加载失败：${snap.error}')));
            }
            final c = snap.data!;
            return SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text('compose · ${c.name}',
                      style: const TextStyle(fontWeight: FontWeight.bold)),
                  const SizedBox(height: 8),
                  _codeBlock(c.compose),
                  if (c.env.isNotEmpty) ...[
                    const SizedBox(height: 12),
                    const Text('.env',
                        style: TextStyle(fontWeight: FontWeight.bold)),
                    const SizedBox(height: 8),
                    _codeBlock(c.env),
                  ],
                  const SizedBox(height: 24),
                ],
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _codeBlock(String text) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Theme.of(context)
            .colorScheme
            .surfaceContainerHighest
            .withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(8),
      ),
      child: SelectableText(text,
          style: const TextStyle(fontFamily: 'monospace', fontSize: 12)),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('应用部署 · ${widget.agent.hostname}')),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<StacksResult>(
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
            final info = r.info;
            final dirWarning = info != null &&
                (!info.dirWritable ||
                    (info.dirError != null && info.dirError!.isNotEmpty));
            return ListView(
              children: [
                if (dirWarning)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(12, 12, 12, 0),
                    child: Material(
                      color: Theme.of(context).colorScheme.errorContainer,
                      borderRadius: BorderRadius.circular(8),
                      child: ListTile(
                        leading: Icon(Icons.warning_amber,
                            color: Theme.of(context).colorScheme.onErrorContainer),
                        title: Text(
                            'stacks 目录异常：${info.dirError?.isNotEmpty == true ? info.dirError : '不可写'}'),
                        subtitle: Text(info.dir),
                      ),
                    ),
                  ),
                if (r.stacks.isEmpty)
                  const Padding(
                    padding: EdgeInsets.all(32),
                    child: Center(child: Text('暂无应用部署')),
                  ),
                for (final s in r.stacks)
                  ListTile(
                    leading: Icon(
                      s.total > 0 && s.running == s.total
                          ? Icons.check_circle
                          : s.running > 0
                              ? Icons.warning_amber
                              : Icons.stop_circle,
                      color: s.total > 0 && s.running == s.total
                          ? Colors.green
                          : s.running > 0
                              ? Colors.orange
                              : Colors.grey,
                    ),
                    title: Text(s.name),
                    subtitle: Text(
                        '${s.running}/${s.total} 服务运行 · 上次 ${s.lastAction.isEmpty ? '-' : s.lastAction}: ${s.lastStatus.isEmpty ? '-' : s.lastStatus}'
                        '${s.lastDeployedAt > 0 ? ' · ${_fmtTime(s.lastDeployedAt)}' : ''}'),
                    isThreeLine: true,
                    trailing: _acting == s.name
                        ? const SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2))
                        : PopupMenuButton<String>(
                            onSelected: (a) {
                              if (a == 'compose') {
                                _showCompose(s.name);
                              } else {
                                _action(s, a);
                              }
                            },
                            itemBuilder: (_) => const [
                              PopupMenuItem(value: 'up', child: Text('启动')),
                              PopupMenuItem(value: 'restart', child: Text('重启')),
                              PopupMenuItem(value: 'down', child: Text('停止')),
                              PopupMenuItem(
                                  value: 'pull', child: Text('拉取镜像')),
                              PopupMenuItem(
                                  value: 'compose', child: Text('查看 compose')),
                            ],
                          ),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }

  String _fmtTime(int unixSeconds) {
    final t = DateTime.fromMillisecondsSinceEpoch(unixSeconds * 1000);
    final mm = t.month.toString().padLeft(2, '0');
    final dd = t.day.toString().padLeft(2, '0');
    final hh = t.hour.toString().padLeft(2, '0');
    final mi = t.minute.toString().padLeft(2, '0');
    return '$mm-$dd $hh:$mi';
  }
}
