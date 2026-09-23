import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// 反代只读视图（M4）：状态卡 + 站点列表 + 配置预览。
/// 写操作（新建/编辑/删除）不在移动端提供——桌面端做。
class ProxyPage extends ConsumerStatefulWidget {
  const ProxyPage({super.key, required this.agent});

  final Agent agent;

  @override
  ConsumerState<ProxyPage> createState() => _ProxyPageState();
}

class _ProxyPageState extends ConsumerState<ProxyPage> {
  late Future<ProxyViewData> _future;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  void _reload() {
    _future = ref.read(apiProvider.future).then((api) async {
      final status = await api.proxyStatus(widget.agent.id);
      final sites = await api.proxySites(widget.agent.id);
      return ProxyViewData(status: status, sites: sites);
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('反代 · ${widget.agent.hostname}')),
      body: RefreshIndicator(
        onRefresh: () async => setState(_reload),
        child: FutureBuilder<ProxyViewData>(
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
            final d = snap.data!;
            final isTraefik = d.status.isTraefik;
            return ListView(
              children: [
                _statusCard(d.status),
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                  child: Text(
                    isTraefik
                        ? '只管理 ${d.status.confDir} 下的 cockpit-site-*.yml 片段，已有配置零接触；'
                            '片段落盘后由 file provider 热加载生效，渲染前经 YAML 自检，失败不落盘。'
                        : '只管理 ${d.status.confDir} 下的 cockpit-site-*.conf 片段，已有配置零接触；'
                            '应用前先 nginx -t 校验，失败不落盘，reload 失败自动回滚。',
                    style: TextStyle(fontSize: 12, color: Colors.grey.shade700),
                  ),
                ),
                if (d.sites.isEmpty)
                  const Padding(
                    padding: EdgeInsets.all(32),
                    child: Center(child: Text('暂无站点')),
                  ),
                for (final s in d.sites) _siteRow(s),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _statusCard(ProxyStatus st) {
    return Card(
      margin: const EdgeInsets.fromLTRB(12, 12, 12, 8),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            _chip(
              '后端 ${st.isTraefik ? 'Traefik' : 'Nginx'}',
              color: st.isTraefik ? Colors.cyan : Colors.green,
            ),
            _chip(
              '版本 ${st.installed ? (st.version.isEmpty ? '已安装' : st.version) : '未检测到'}',
              color: Colors.blueGrey,
            ),
            _chip('目录 ${st.confDir}', color: Colors.blueGrey),
            _chip('站点 ${st.siteCount}', color: Colors.blueGrey),
            _chip('生效 ${st.reloadModeLabel}', color: Colors.blueGrey),
          ],
        ),
      ),
    );
  }

  Widget _chip(String label, {required MaterialColor color}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.4)),
      ),
      child: Text(label,
          style: TextStyle(fontSize: 12, color: color.shade700)),
    );
  }

  Widget _siteRow(ProxySite s) {
    return ListTile(
      title: Text(s.name),
      subtitle: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 4,
            runSpacing: 4,
            children: [
              for (final d in s.serverNames)
                Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: Colors.blue.shade50,
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(d,
                      style:
                          TextStyle(fontSize: 12, color: Colors.blue.shade700)),
                ),
            ],
          ),
          const SizedBox(height: 4),
          Text(s.upstream,
              style: TextStyle(fontSize: 13, color: Colors.grey.shade700)),
        ],
      ),
      trailing: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          Text(
            s.scheme == 'https' ? 'HTTPS' : 'HTTP',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.bold,
              color: s.scheme == 'https' ? Colors.green : Colors.blue,
            ),
          ),
          if (s.websocket)
            Text('WS',
                style: TextStyle(fontSize: 11, color: Colors.purple.shade700)),
        ],
      ),
      onTap: () => _showPreview(s.name),
    );
  }

  Future<void> _showPreview(String name) async {
    final api = await ref.read(apiProvider.future);
    final ProxySiteDetail detail;
    try {
      detail = await api.proxySite(widget.agent.id, name);
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('读取配置失败：$e')));
      return;
    }
    if (!mounted) return;
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (sheetContext) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('配置预览：$name',
                  style: const TextStyle(fontWeight: FontWeight.bold)),
              const SizedBox(height: 8),
              Flexible(
                child: SingleChildScrollView(
                  child: SelectableText(
                    detail.content,
                    style: const TextStyle(
                        fontFamily: 'monospace', fontSize: 12),
                  ),
                ),
              ),
              const SizedBox(height: 8),
              Align(
                alignment: Alignment.centerRight,
                child: TextButton(
                  onPressed: () => Navigator.of(sheetContext).pop(),
                  child: const Text('关闭'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
