import 'package:dio/dio.dart';
import 'package:flutter/material.dart';

import '../models/models.dart';

/// 反代站点新建/编辑表单（M5）。
/// 字段与校验严格对齐 web pages/Proxy 表单（正则/错文案/trim 语义逐字段核对）。
class ProxyEditor extends StatefulWidget {
  const ProxyEditor({
    super.key,
    required this.editing,
    required this.isTraefik,
    required this.onSave,
  });

  /// 非空 = 编辑（name 禁用），null = 新建。
  final ProxySite? editing;
  final bool isTraefik;
  final Future<void> Function(ProxySite site) onSave;

  @override
  State<ProxyEditor> createState() => _ProxyEditorState();
}

class _ProxyEditorState extends State<ProxyEditor> {
  static final _nameRe = RegExp(r'^[a-z0-9][a-z0-9_-]{0,63}$');
  static final _domainRe = RegExp(r'^[A-Za-z0-9*.\-]{1,253}$');
  static final _upstreamRe = RegExp(r'^[A-Za-z0-9.:\-]{1,253}$');

  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _name;
  late final TextEditingController _upstream;
  late final TextEditingController _tlsCert;
  late final TextEditingController _tlsKey;
  late final TextEditingController _extra;
  final _domainCtrl = TextEditingController();
  late List<String> _serverNames;
  late String _scheme;
  late bool _websocket;
  String? _applyError;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final e = widget.editing;
    _name = TextEditingController(text: e?.name ?? '');
    _upstream = TextEditingController(text: e?.upstream ?? '');
    _tlsCert = TextEditingController(text: e?.tlsCert ?? '');
    _tlsKey = TextEditingController(text: e?.tlsKey ?? '');
    _extra = TextEditingController(text: e?.extra ?? '');
    _serverNames = [...?e?.serverNames];
    _scheme = e?.scheme ?? 'http';
    _websocket = e?.websocket ?? false;
  }

  @override
  void dispose() {
    _name.dispose();
    _upstream.dispose();
    _tlsCert.dispose();
    _tlsKey.dispose();
    _extra.dispose();
    _domainCtrl.dispose();
    super.dispose();
  }

  void _addDomain() {
    final v = _domainCtrl.text.trim();
    if (v.isEmpty) return;
    setState(() {
      _serverNames.add(v);
      _domainCtrl.clear();
    });
  }

  Future<void> _submit() async {
    setState(() => _applyError = null);
    if (!(_formKey.currentState?.validate() ?? false)) return;
    // trim 语义对齐 web（Go 不 trim）：serverNames 逐项 trim 后 filter(Boolean)
    final names = _serverNames
        .map((e) => e.trim())
        .where((e) => e.isNotEmpty)
        .toList();
    if (names.isEmpty) {
      setState(() => _applyError = '请至少填写一个域名');
      return;
    }
    final site = ProxySite(
      name: _name.text.trim(),
      serverNames: names,
      upstream: _upstream.text.trim(),
      scheme: _scheme,
      tlsCert: _scheme == 'https' ? _tlsCert.text.trim() : null,
      tlsKey: _scheme == 'https' ? _tlsKey.text.trim() : null,
      websocket: _websocket,
      // extra 空串归一为不传（toPayload 按 isNotEmpty 过滤）
      extra: _extra.text.trim(),
    );
    setState(() => _saving = true);
    try {
      await widget.onSave(site);
      if (mounted) Navigator.of(context).pop(site);
    } catch (e) {
      // 应用失败不关 Modal，错误原文呈现在表单内 Alert（对齐 web）
      setState(() => _applyError = _extractError(e));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  static String _extractError(Object e) {
    // 对齐 web getApiErrorMessage：取响应体 error 字段原文
    if (e is DioException) {
      final data = e.response?.data;
      if (data is Map && data['error'] != null) {
        return data['error'].toString();
      }
    }
    return e.toString();
  }

  @override
  Widget build(BuildContext context) {
    final editing = widget.editing != null;
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.only(
          left: 16,
          right: 16,
          top: 16,
          bottom: MediaQuery.of(context).viewInsets.bottom + 16,
        ),
        child: Form(
          key: _formKey,
          child: SingleChildScrollView(
            child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(editing ? '编辑站点 ${widget.editing!.name}' : '新建站点',
                  style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 16)),
              if (_applyError != null) ...[
                const SizedBox(height: 12),
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color: Theme.of(context).colorScheme.errorContainer,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('应用失败',
                          style: TextStyle(
                              fontWeight: FontWeight.bold,
                              color: Theme.of(context)
                                  .colorScheme
                                  .onErrorContainer)),
                      const SizedBox(height: 4),
                      SelectableText(_applyError!,
                          style: TextStyle(
                              fontFamily: 'monospace',
                              fontSize: 12,
                              color: Theme.of(context)
                                  .colorScheme
                                  .onErrorContainer)),
                    ],
                  ),
                ),
              ],
              const SizedBox(height: 16),
              TextFormField(
                controller: _name,
                enabled: !editing,
                decoration: const InputDecoration(
                  labelText: '站点名称',
                  helperText: '用于配置文件名 cockpit-site-<名称>.conf，创建后不可修改',
                  border: OutlineInputBorder(),
                ),
                validator: (v) {
                  final s = (v ?? '').trim();
                  if (s.isEmpty) return '请输入站点名称';
                  if (!_nameRe.hasMatch(s)) {
                    return '小写字母/数字开头，可用 - 和 _，最长 64 字符';
                  }
                  return null;
                },
              ),
              const SizedBox(height: 16),
              _domainField(),
              const SizedBox(height: 16),
              TextFormField(
                controller: _upstream,
                decoration: const InputDecoration(
                  labelText: '上游地址',
                  hintText: '127.0.0.1:3000',
                  border: OutlineInputBorder(),
                ),
                validator: (v) {
                  final s = (v ?? '').trim();
                  if (s.isEmpty) return '请输入上游地址';
                  if (!_upstreamRe.hasMatch(s)) return '形如 127.0.0.1:3000';
                  return null;
                },
              ),
              const SizedBox(height: 16),
              const Text('协议', style: TextStyle(fontWeight: FontWeight.bold)),
              const SizedBox(height: 8),
              SegmentedButton<String>(
                segments: const [
                  ButtonSegment(value: 'http', label: Text('HTTP')),
                  ButtonSegment(value: 'https', label: Text('HTTPS')),
                ],
                selected: {_scheme},
                onSelectionChanged: (s) => setState(() => _scheme = s.first),
              ),
              if (_scheme == 'https') ...[
                const SizedBox(height: 16),
                TextFormField(
                  controller: _tlsCert,
                  decoration: const InputDecoration(
                    labelText: '证书路径',
                    helperText:
                        '可经工作台 - 文件上传，或在「证书签发」页配置自动部署（默认推送到 /etc/cockpit/certs/，续期自动更新）',
                    hintText: '/etc/cockpit/certs/site.crt.pem',
                    border: OutlineInputBorder(),
                  ),
                  validator: (v) => _scheme == 'https' && (v ?? '').trim().isEmpty
                      ? 'https 需要证书绝对路径'
                      : null,
                ),
                const SizedBox(height: 16),
                TextFormField(
                  controller: _tlsKey,
                  decoration: const InputDecoration(
                    labelText: '私钥路径',
                    hintText: '/etc/ssl/private/site.key',
                    border: OutlineInputBorder(),
                  ),
                  validator: (v) => _scheme == 'https' && (v ?? '').trim().isEmpty
                      ? 'https 需要私钥绝对路径'
                      : null,
                ),
              ],
              const SizedBox(height: 8),
              SwitchListTile(
                title: const Text('WebSocket 支持'),
                value: _websocket,
                contentPadding: EdgeInsets.zero,
                onChanged: (v) => setState(() => _websocket = v),
              ),
              const SizedBox(height: 8),
              TextFormField(
                controller: _extra,
                maxLines: 4,
                enabled: !widget.isTraefik,
                style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
                decoration: InputDecoration(
                  labelText: '高级指令',
                  helperText: widget.isTraefik
                      ? 'Traefik 后端不支持高级指令，留空即可'
                      : '原样插入 server 块内，如 client_max_body_size 50m；语法由 nginx -t 校验兜底',
                  hintText: 'client_max_body_size 50m;',
                  border: const OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 16),
              FilledButton(
                onPressed: _saving ? null : _submit,
                child: _saving
                    ? const SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2))
                    : Text(editing ? '保存' : '创建'),
              ),
              const SizedBox(height: 8),
            ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _domainField() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('域名', style: TextStyle(fontWeight: FontWeight.bold)),
        const SizedBox(height: 4),
        Text('支持通配符如 *.example.com，回车确认，最多 16 个',
            style: TextStyle(fontSize: 12, color: Colors.grey.shade700)),
        const SizedBox(height: 8),
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: [
            for (final d in _serverNames)
              InputChip(
                label: Text(d),
                onDeleted: () => setState(() => _serverNames.remove(d)),
              ),
          ],
        ),
        const SizedBox(height: 8),
        Row(
          children: [
            Expanded(
              child: TextFormField(
                controller: _domainCtrl,
                decoration: const InputDecoration(
                  hintText: 'blog.example.com',
                  border: OutlineInputBorder(),
                  isDense: true,
                ),
                onFieldSubmitted: (_) => _addDomain(),
              ),
            ),
            const SizedBox(width: 8),
            IconButton.filled(
              onPressed: _addDomain,
              tooltip: '添加域名',
              icon: const Icon(Icons.playlist_add),
            ),
          ],
        ),
        if (_serverNames.isEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text('请输入至少一个域名',
                style: TextStyle(
                    fontSize: 12,
                    color: Theme.of(context).colorScheme.error)),
          ) else if (_serverNames
              .any((d) => !_domainRe.hasMatch(d.trim())))
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text('域名只能含字母/数字/点/连字符/通配符 *',
                style: TextStyle(
                    fontSize: 12,
                    color: Theme.of(context).colorScheme.error)),
          ),
      ],
    );
  }
}
