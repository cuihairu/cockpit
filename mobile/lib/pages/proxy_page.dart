import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';
import 'proxy_editor.dart';

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
  bool _isTraefik = false;

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
      appBar: AppBar(
        title: Text('反代 · ${widget.agent.hostname}'),
        actions: [
          IconButton(
            icon: const Icon(Icons.add),
            tooltip: '新建站点',
            onPressed: () => _showEditor(null, _isTraefik),
          ),
        ],
      ),
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
            _isTraefik = isTraefik;
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
                for (final s in d.sites) _siteRow(s, isTraefik),
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

  Widget _siteRow(ProxySite s, bool isTraefik) {
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
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Column(
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
                    style:
                        TextStyle(fontSize: 11, color: Colors.purple.shade700)),
            ],
          ),
          IconButton(
            icon: const Icon(Icons.edit, size: 20),
            tooltip: '编辑',
            onPressed: () => _showEditor(s.name, isTraefik),
          ),
          IconButton(
            icon: const Icon(Icons.delete, size: 20, color: Colors.red),
            tooltip: '删除',
            onPressed: () => _confirmDelete(s.name, isTraefik),
          ),
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

  /// 新建/编辑表单（M5）：editing 传站点名 = 编辑（拉详情预填，列表是裁剪版
  /// 无 tlsCert/tlsKey/extra，必须走 site.get 才能预填全量）。
  Future<void> _showEditor(String? name, bool isTraefik) async {
    final api = await ref.read(apiProvider.future);
    ProxySite? editing;
    if (name != null) {
      try {
        editing = (await api.proxySite(widget.agent.id, name)).site;
      } catch (e) {
        if (!mounted) return;
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('读取站点失败：$e')));
        return;
      }
    }
    if (!mounted) return;
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (sheetContext) => ProxyEditor(
        editing: editing,
        isTraefik: isTraefik,
        onSave: (site) async {
          await api.applyProxySite(widget.agent.id, site.name, site.toPayload());
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text('站点 ${site.name} 已应用')));
          }
          setState(_reload);
        },
      ),
    );
  }

  Future<void> _confirmDelete(String name, bool isTraefik) async {
    final backendName = isTraefik ? 'Traefik' : 'Nginx';
    final ok = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('删除站点'),
        content: Text('将从 $backendName 移除 $name，确认？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            style: TextButton.styleFrom(foregroundColor: Colors.red),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    final api = await ref.read(apiProvider.future);
    try {
      await api.deleteProxySite(widget.agent.id, name);
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('站点 $name 已删除')));
      setState(_reload);
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('删除失败：$e')));
    }
  }}
