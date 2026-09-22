import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// 定时任务只读视图（M2）：cockpit 任务 + 外部条目原文。
/// 写操作（增删改 cron job）不在移动端提供——桌面端做。
class CronJobsPage extends ConsumerStatefulWidget {
  const CronJobsPage({super.key, required this.agent});

  final Agent agent;

  @override
  ConsumerState<CronJobsPage> createState() => _CronJobsPageState();
}

class _CronJobsPageState extends ConsumerState<CronJobsPage> {
  late Future<CronJobsResult> _future;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future = ref
        .read(apiProvider.future)
        .then((api) => api.cronJobs(widget.agent.id));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('定时任务 · ${widget.agent.hostname}')),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<CronJobsResult>(
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
            return ListView(
              children: [
                if (r.jobs.isEmpty && r.external.isEmpty)
                  const Padding(
                    padding: EdgeInsets.all(32),
                    child: Center(child: Text('暂无定时任务')),
                  ),
                for (final j in r.jobs)
                  ListTile(
                    leading: Icon(
                      j.enabled ? Icons.schedule : Icons.schedule_outlined,
                      color: j.enabled ? Colors.green : Colors.grey,
                    ),
                    title: Text(j.name),
                    subtitle: Text('${j.schedule}\n${j.command}'),
                    isThreeLine: true,
                    trailing: j.nextRun > 0
                        ? Text('下次 ${_fmtTime(j.nextRun)}',
                            style: const TextStyle(fontSize: 12))
                        : null,
                  ),
                if (r.external.isNotEmpty) ...[
                  const Divider(),
                  const Padding(
                    padding: EdgeInsets.fromLTRB(16, 8, 16, 4),
                    child: Text('外部条目（只读）',
                        style: TextStyle(fontWeight: FontWeight.bold)),
                  ),
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
                    child: Container(
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
                        r.external,
                        style: const TextStyle(
                            fontFamily: 'monospace', fontSize: 12),
                      ),
                    ),
                  ),
                ],
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
