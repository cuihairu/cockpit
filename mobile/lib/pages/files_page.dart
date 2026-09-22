import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// 远程文件只读浏览（M2）：目录导航 + 条目详情。
/// 写操作（写/删/改名/chmod）与文件内容查看不在移动端提供——桌面端做。
class FilesPage extends ConsumerStatefulWidget {
  const FilesPage({super.key, required this.agent, this.initialDir = '/'});

  final Agent agent;
  final String initialDir;

  @override
  ConsumerState<FilesPage> createState() => _FilesPageState();
}

class _FilesPageState extends ConsumerState<FilesPage> {
  late String _dir;
  final _stack = <String>[]; // 目录回退栈
  late Future<FileListResult> _future;

  @override
  void initState() {
    super.initState();
    _dir = widget.initialDir;
    _reload();
  }

  void _reload() {
    _future = ref
        .read(apiProvider.future)
        .then((api) => api.listFiles(widget.agent.id, _dir));
  }

  void _enter(FileEntry e) {
    final child = _dir == '/' ? '/${e.name}' : '$_dir/${e.name}';
    setState(() {
      _stack.add(_dir);
      _dir = child;
      _reload();
    });
  }

  void _goUp() {
    if (_stack.isEmpty) return;
    setState(() {
      _dir = _stack.removeLast();
      _reload();
    });
  }

  void _showEntry(FileEntry e) {
    final path = _dir == '/' ? '/${e.name}' : '$_dir/${e.name}';
    showModalBottomSheet<void>(
      context: context,
      builder: (_) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(e.name,
                  style: const TextStyle(
                      fontWeight: FontWeight.bold, fontSize: 16)),
              const SizedBox(height: 8),
              Text('路径：$path'),
              Text('类型：${e.isDir ? '目录' : e.isSymlink ? '链接' : '文件'}'),
              if (e.isSymlink && e.target.isNotEmpty) Text('指向：${e.target}'),
              Text('大小：${_fmtSize(e.size)}'),
              if (e.mode.isNotEmpty) Text('权限：${e.mode}'),
              if (e.uid >= 0) Text('属主：${e.uid}:${e.gid}'),
              Text('修改时间：${_fmtTime(e.mtime)}'),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return PopScope(
      canPop: _stack.isEmpty,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) _goUp();
      },
      child: Scaffold(
        appBar: AppBar(title: Text('文件 · ${_dir == '/' ? '/' : _dir.split('/').last}')),
        body: RefreshIndicator(
          onRefresh: () async => setState(_reload),
          child: FutureBuilder<FileListResult>(
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
              final entries = [...snap.data!.entries]
                ..sort((a, b) {
                  if (a.isDir != b.isDir) return a.isDir ? -1 : 1;
                  return a.name.compareTo(b.name);
                });
              if (entries.isEmpty) {
                return ListView(children: const [
                  SizedBox(height: 80),
                  Center(child: Text('空目录')),
                ]);
              }
              return ListView.separated(
                itemCount: entries.length,
                separatorBuilder: (_, _) => const Divider(height: 1),
                itemBuilder: (context, i) {
                  final e = entries[i];
                  final icon = e.isDir
                      ? Icons.folder
                      : e.isSymlink
                          ? Icons.link
                          : Icons.insert_drive_file_outlined;
                  final color = e.isDir
                      ? Colors.amber.shade700
                      : e.isSymlink
                          ? Colors.blue
                          : Colors.grey;
                  return ListTile(
                    leading: Icon(icon, color: color),
                    title: Text(e.name,
                        maxLines: 1, overflow: TextOverflow.ellipsis),
                    subtitle: Text(
                        '${e.isDir ? '目录' : _fmtSize(e.size)}'
                        '${e.mode.isEmpty ? '' : ' · ${e.mode}'}'
                        ' · ${_fmtTime(e.mtime)}',
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis),
                    trailing: e.isDir
                        ? const Icon(Icons.chevron_right)
                        : null,
                    onTap: e.isDir ? () => _enter(e) : () => _showEntry(e),
                  );
                },
              );
            },
          ),
        ),
      ),
    );
  }

  String _fmtSize(int bytes) {
    // 与 web 端 formatBytes 同规则：B 整数、KB/MB 一位小数、GB/TB 两位小数
    if (bytes <= 0) return '0 B';
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) {
      return '${(bytes / 1024 / 1024).toStringAsFixed(1)} MB';
    }
    if (bytes < 1024 * 1024 * 1024 * 1024) {
      return '${(bytes / 1024 / 1024 / 1024).toStringAsFixed(2)} GB';
    }
    return '${(bytes / 1024 / 1024 / 1024 / 1024).toStringAsFixed(2)} TB';
  }

  String _fmtTime(int unixSeconds) {
    if (unixSeconds <= 0) return '—';
    final t = DateTime.fromMillisecondsSinceEpoch(unixSeconds * 1000);
    final mm = t.month.toString().padLeft(2, '0');
    final dd = t.day.toString().padLeft(2, '0');
    final hh = t.hour.toString().padLeft(2, '0');
    final mi = t.minute.toString().padLeft(2, '0');
    return '$mm-$dd $hh:$mi';
  }
}
