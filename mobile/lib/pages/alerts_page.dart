import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/models.dart';
import '../state/settings.dart';

/// 告警列表 + 全部已读（PUT /alerts/read-all）。
class AlertsPage extends ConsumerStatefulWidget {
  const AlertsPage({super.key});

  @override
  ConsumerState<AlertsPage> createState() => _AlertsPageState();
}

class _AlertsPageState extends ConsumerState<AlertsPage> {
  late Future<List<Alert>> _future;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future = ref.read(apiProvider.future).then((api) => api.alerts());
  }

  Future<void> _readAll() async {
    try {
      final api = await ref.read(apiProvider.future);
      await api.readAllAlerts();
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('操作失败：$e')));
    } finally {
      if (mounted) {
        _reload();
        setState(() {});
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('告警'),
        actions: [
          TextButton(onPressed: _readAll, child: const Text('全部已读')),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<List<Alert>>(
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
            final list = snap.data ?? const <Alert>[];
            if (list.isEmpty) {
              return ListView(children: const [
                SizedBox(height: 80),
                Center(child: Text('暂无告警')),
              ]);
            }
            return ListView.separated(
              itemCount: list.length,
              separatorBuilder: (_, _) => const Divider(height: 1),
              itemBuilder: (context, i) {
                final a = list[i];
                final color = switch (a.type) {
                  'error' => Colors.red,
                  'warning' => Colors.orange,
                  'success' => Colors.green,
                  _ => Colors.blue,
                };
                return ListTile(
                  leading: Icon(_icon(a.type), color: color),
                  title: Text(a.title,
                      style: TextStyle(
                          fontWeight: a.read
                              ? FontWeight.normal
                              : FontWeight.bold)),
                  subtitle: Text(a.message, maxLines: 2,
                      overflow: TextOverflow.ellipsis),
                  trailing: a.read
                      ? null
                      : const Icon(Icons.circle,
                          size: 8, color: Colors.blue),
                );
              },
            );
          },
        ),
      ),
    );
  }

  IconData _icon(String type) => switch (type) {
        'error' => Icons.error_outline,
        'warning' => Icons.warning_amber_outlined,
        'success' => Icons.check_circle_outline,
        _ => Icons.info_outline,
      };
}
