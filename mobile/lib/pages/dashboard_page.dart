import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/settings.dart';

/// 仪表盘：agent 在线/离线、未读告警、error 告警三卡片（M1 只读）。
class DashboardPage extends ConsumerWidget {
  const DashboardPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final apiAsync = ref.watch(apiProvider);
    return apiAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => _ErrorRetry(message: e.toString()),
      data: (api) => RefreshIndicator(
        onRefresh: () async {},
        child: FutureBuilder(
          future: Future.wait([api.agents(), api.alerts()]),
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
            final agents = snap.data![0] as List;
            final alerts = snap.data![1] as List;
            final online = agents.where((a) => a.online).length;
            final unread = alerts.where((a) => !a.read).length;
            final errors = alerts
                .where((a) => !a.read && a.type == 'error')
                .length;
            return ListView(
              padding: const EdgeInsets.all(12),
              children: [
                Row(children: [
                  _Card(
                      label: '主机在线',
                      value: '$online',
                      sub: '/ ${agents.length}',
                      color: Theme.of(context).colorScheme.primary),
                  const SizedBox(width: 12),
                  _Card(
                      label: '未读告警',
                      value: '$unread',
                      sub: unread > 0 ? '需关注' : '清爽',
                      color: unread > 0
                          ? Theme.of(context).colorScheme.tertiary
                          : Colors.green),
                ]),
                const SizedBox(height: 12),
                Row(children: [
                  _Card(
                      label: '错误告警',
                      value: '$errors',
                      sub: errors > 0 ? '尽快处理' : '无',
                      color: errors > 0 ? Colors.red : Colors.green),
                ]),
              ],
            );
          },
        ),
      ),
    );
  }
}

class _Card extends StatelessWidget {
  const _Card(
      {required this.label,
      required this.value,
      required this.sub,
      required this.color});

  final String label;
  final String value;
  final String sub;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(label,
                  style: Theme.of(context).textTheme.labelMedium),
              const SizedBox(height: 6),
              Row(
                crossAxisAlignment: CrossAxisAlignment.baseline,
                textBaseline: TextBaseline.alphabetic,
                children: [
                  Text(value,
                      style: Theme.of(context)
                          .textTheme
                          .headlineLarge
                          ?.apply(color: color)),
                  const SizedBox(width: 4),
                  Text(sub, style: Theme.of(context).textTheme.bodySmall),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ErrorRetry extends ConsumerWidget {
  const _ErrorRetry({required this.message});

  final String message;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(message, textAlign: TextAlign.center),
          const SizedBox(height: 8),
          OutlinedButton(
            onPressed: () => ref.invalidate(apiProvider),
            child: const Text('重试'),
          ),
        ],
      ),
    );
  }
}
