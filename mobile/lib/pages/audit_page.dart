import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// 审计日志（admin/audit/logs）：分页列表 + 加载更多。
class AuditPage extends ConsumerStatefulWidget {
  const AuditPage({super.key});

  @override
  ConsumerState<AuditPage> createState() => _AuditPageState();
}

class _AuditPageState extends ConsumerState<AuditPage> {
  static const _pageSize = 20;

  final _logs = <AuditLog>[];
  int _page = 1;
  int _total = 0;
  bool _loading = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _loadMore(reset: true);
  }

  Future<void> _loadMore({bool reset = false}) async {
    if (_loading) return;
    setState(() {
      _loading = true;
      _error = null;
      if (reset) {
        _page = 1;
        _logs.clear();
        _total = 0;
      }
    });
    try {
      final api = await ref.read(apiProvider.future);
      final page = await api.auditLogs(page: _page, pageSize: _pageSize);
      if (!mounted) return;
      setState(() {
        _logs.addAll(page.data);
        _total = page.total;
        _page += 1;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  bool get _hasMore => _logs.length < _total;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('审计日志')),
      body: RefreshIndicator(
        onRefresh: () => _loadMore(reset: true),
        child: _error != null && _logs.isEmpty
            ? ListView(children: [
                const SizedBox(height: 80),
                Center(child: Text('加载失败：$_error')),
              ])
            : _logs.isEmpty && !_loading
                ? ListView(children: const [
                    SizedBox(height: 80),
                    Center(child: Text('暂无审计记录')),
                  ])
                : ListView.separated(
                    itemCount: _logs.length + (_hasMore || _loading ? 1 : 0),
                    separatorBuilder: (_, _) => const Divider(height: 1),
                    itemBuilder: (context, i) {
                      if (i >= _logs.length) {
                        if (_hasMore) {
                          // 不能在 build 期间 setState，延后一帧再拉下一页。
                          WidgetsBinding.instance
                              .addPostFrameCallback((_) => _loadMore());
                        }
                        return const Padding(
                          padding: EdgeInsets.all(16),
                          child: Center(
                              child: SizedBox(
                                  width: 18,
                                  height: 18,
                                  child: CircularProgressIndicator(
                                      strokeWidth: 2))),
                        );
                      }
                      final log = _logs[i];
                      final failed = log.status != 'success' &&
                          log.status != 'ok' &&
                          log.status.isNotEmpty;
                      return ListTile(
                        leading: Icon(
                          failed ? Icons.error_outline : Icons.history,
                          color: failed ? Colors.red : null,
                        ),
                        title: Text('${log.username} · ${log.action}'),
                        subtitle: Text(
                          '${log.resource}${log.resourceId.isEmpty ? '' : ' #${log.resourceId}'}'
                          '  ${log.ip}\n${log.createdAt}',
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        isThreeLine: true,
                      );
                    },
                  ),
      ),
    );
  }
}
