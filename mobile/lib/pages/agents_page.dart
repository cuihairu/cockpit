import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/models.dart';
import '../state/settings.dart';
import 'cron_page.dart';
import 'files_page.dart';

/// 主机列表：hostname · region/zone · 在线徽标 · docker/cron/file 能力入口。
class AgentsPage extends ConsumerStatefulWidget {
  const AgentsPage({super.key});

  @override
  ConsumerState<AgentsPage> createState() => _AgentsPageState();
}

class _AgentsPageState extends ConsumerState<AgentsPage> {
  late Future<List<Agent>> _future;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future =
        ref.read(apiProvider.future).then((api) => api.agents());
  }

  /// 能力动作单：行点开的完整入口（含终端），trailing 图标是高频捷径。
  void _showActions(Agent a) {
    showModalBottomSheet<void>(
      context: context,
      builder: (sheetContext) => SafeArea(
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          const Padding(
            padding: EdgeInsets.all(12),
            child: Text('选择功能',
                style: TextStyle(fontWeight: FontWeight.bold)),
          ),
          if (a.hasDocker)
            ListTile(
              leading: const Icon(Icons.view_in_ar),
              title: const Text('容器'),
              onTap: () => _push(sheetContext, ContainersPage(agent: a)),
            ),
          if (a.hasCron)
            ListTile(
              leading: const Icon(Icons.schedule),
              title: const Text('定时任务'),
              onTap: () => _push(sheetContext, CronJobsPage(agent: a)),
            ),
          if (a.hasFile)
            ListTile(
              leading: const Icon(Icons.folder_open),
              title: const Text('文件'),
              onTap: () => _push(sheetContext, FilesPage(agent: a)),
            ),
          const SizedBox(height: 8),
        ]),
      ),
    );
  }

  void _push(BuildContext sheetContext, Widget page) {
    Navigator.of(sheetContext).pop();
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => page),
    );
  }

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: () async => setState(_reload),
      child: FutureBuilder<List<Agent>>(
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
          final agents = snap.data ?? const <Agent>[];
          if (agents.isEmpty) {
            return ListView(children: const [
              SizedBox(height: 80),
              Center(child: Text('暂无已注册主机')),
            ]);
          }
          return ListView.separated(
            itemCount: agents.length,
            separatorBuilder: (_, _) => const Divider(height: 1),
            itemBuilder: (context, i) {
              final a = agents[i];
              final loc = [
                if (a.region != null && a.region!.isNotEmpty) a.region!,
                if (a.zone != null && a.zone!.isNotEmpty) a.zone!,
              ].join(' · ');
              return ListTile(
                leading: Icon(
                  a.online ? Icons.check_circle : Icons.cancel,
                  color: a.online ? Colors.green : Colors.red,
                ),
                title: Text(a.hostname),
                subtitle: Text(loc.isEmpty ? a.ip : '${a.ip} · $loc'),
                onTap: () => _showActions(a),
                trailing: Row(mainAxisSize: MainAxisSize.min, children: [
                  if (a.hasDocker)
                    IconButton(
                      icon: const Icon(Icons.view_in_ar),
                      tooltip: '容器',
                      onPressed: () => Navigator.of(context).push(
                        MaterialPageRoute(
                          builder: (_) => ContainersPage(agent: a),
                        ),
                      ),
                    ),
                  if (a.hasCron)
                    IconButton(
                      icon: const Icon(Icons.schedule),
                      tooltip: '定时任务',
                      onPressed: () => Navigator.of(context).push(
                        MaterialPageRoute(
                          builder: (_) => CronJobsPage(agent: a),
                        ),
                      ),
                    ),
                  if (a.hasFile)
                    IconButton(
                      icon: const Icon(Icons.folder_open),
                      tooltip: '文件',
                      onPressed: () => Navigator.of(context).push(
                        MaterialPageRoute(
                          builder: (_) => FilesPage(agent: a),
                        ),
                      ),
                    ),
                ]),
              );
            },
          );
        },
      ),
    );
  }
}

/// 容器列表 + start/stop/restart（M1 核心写操作）。
class ContainersPage extends ConsumerStatefulWidget {
  const ContainersPage({super.key, required this.agent});

  final Agent agent;

  @override
  ConsumerState<ContainersPage> createState() => _ContainersPageState();
}

class _ContainersPageState extends ConsumerState<ContainersPage> {
  late Future<List<ContainerInfo>> _future;
  String? _acting;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future = ref
        .read(apiProvider.future)
        .then((api) => api.containers(widget.agent.id));
  }

  Future<void> _action(ContainerInfo c, String action) async {
    setState(() => _acting = '${c.id}:$action');
    try {
      final api = await ref.read(apiProvider.future);
      await api.containerAction(widget.agent.id, c.id, action);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('${c.name} $action 完成')));
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('${c.name} $action 失败：$e')));
    } finally {
      if (mounted) {
        setState(() => _acting = null);
        _reload();
        setState(() {});
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('容器 · ${widget.agent.hostname}')),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<List<ContainerInfo>>(
          future: _future,
          builder: (context, snap) {
            if (snap.connectionState != ConnectionState.done &&
                _acting == null) {
              return const Center(child: CircularProgressIndicator());
            }
            if (snap.hasError) {
              return ListView(children: [
                const SizedBox(height: 80),
                Center(child: Text('加载失败：${snap.error}')),
              ]);
            }
            final list = snap.data ?? const <ContainerInfo>[];
            return ListView.separated(
              itemCount: list.length,
              separatorBuilder: (_, _) => const Divider(height: 1),
              itemBuilder: (context, i) {
                final c = list[i];
                final running = c.state == 'running';
                return ListTile(
                  leading: Icon(
                    running
                        ? Icons.play_circle
                        : c.state == 'paused'
                            ? Icons.pause_circle
                            : Icons.stop_circle,
                    color: running ? Colors.green : Colors.grey,
                  ),
                  title: Text(c.name),
                  subtitle: Text('${c.image}\n${c.status}'),
                  isThreeLine: true,
                  trailing: _acting != null
                      ? const SizedBox(
                          width: 18, height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2))
                      : PopupMenuButton<String>(
                          onSelected: (a) => _action(c, a),
                          itemBuilder: (_) => [
                            const PopupMenuItem(
                                value: 'start', child: Text('启动')),
                            if (running)
                              const PopupMenuItem(
                                  value: 'stop', child: Text('停止')),
                            if (running)
                              const PopupMenuItem(
                                  value: 'restart', child: Text('重启')),
                          ],
                        ),
                );
              },
            );
          },
        ),
      ),
    );
  }
}
