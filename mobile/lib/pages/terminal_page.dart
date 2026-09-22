import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'package:xterm/xterm.dart';

import '../api/endpoints.dart';
import '../models/models.dart';
import '../state/settings.dart';

/// SSH 终端（M2/D7）：POST tickets 换票据 → WS 子协议握手 → xterm 双向转发。
/// 协议对齐 web TerminalModal：下行 {type:data|error|close}，上行 {type:input}。
class TerminalPage extends ConsumerStatefulWidget {
  const TerminalPage({super.key, required this.agent, required this.ssh});

  final Agent agent;
  final SshService ssh;

  @override
  ConsumerState<TerminalPage> createState() => _TerminalPageState();
}

enum _ConnState { connecting, connected, error }

class _TerminalPageState extends ConsumerState<TerminalPage> {
  final _terminal = Terminal(maxLines: 5000);
  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _sub;
  _ConnState _state = _ConnState.connecting;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _connect();
  }

  Future<void> _connect() async {
    setState(() {
      _state = _ConnState.connecting;
      _error = '';
    });
    try {
      final api = await ref.read(apiProvider.future);
      final ticket = await api.createRemoteTicket(
        agentId: widget.agent.id,
        host: widget.ssh.host,
        port: widget.ssh.port,
      );
      final channel = await _openChannel(ticket.ticket);
      if (!mounted) {
        await channel.sink.close();
        return;
      }
      setState(() => _state = _ConnState.connected);
      _terminal.write('连接成功：${widget.ssh.host}:${widget.ssh.port}\r\n');
      _channel = channel;
      _terminal.onOutput = (data) {
        channel.sink.add(jsonEncode({'type': 'input', 'data': data}));
      };
      // 保存 subscription 以便 dispose 时取消：WS 回调落在真 async 区，
      // 测试/页面销毁后仍会触发 _fail → setState(defunct)。
      _sub = channel.stream.listen(
        (msg) => _onServerMessage(msg as String),
        onError: (Object e) => _fail('连接错误：$e'),
        onDone: () => _fail('连接已关闭'),
      );
    } catch (e) {
      _fail('$e');
    }
  }

  /// ticket 作 WS 子协议；自签名开关同样作用于 wss。
  Future<WebSocketChannel> _openChannel(String ticket) async {
    final server = Uri.parse(ref.read(settingsProvider).serverUrl);
    final wsScheme = server.scheme == 'https' ? 'wss' : 'ws';
    final uri = server.replace(
      scheme: wsScheme,
      path: '/api/remote/terminal',
    );
    if (server.scheme != 'https') {
      return IOWebSocketChannel.connect(uri, protocols: [ticket]);
    }
    final allowSelfSigned = ref.read(settingsProvider).allowSelfSigned;
    final client = HttpClient()
      ..badCertificateCallback = allowSelfSigned ? (_, _, _) => true : null;
    return IOWebSocketChannel.connect(
      uri,
      protocols: [ticket],
      customClient: client,
    );
  }

  void _onServerMessage(String raw) {
    Object? parsed;
    try {
      parsed = jsonDecode(raw);
    } catch (_) {
      return;
    }
    if (parsed is! Map<String, dynamic>) return;
    switch (parsed['type']) {
      case 'data':
        _terminal.write(parsed['data'] as String? ?? '');
      case 'error':
        _terminal.write('\r\n错误: ${parsed['message']}\r\n');
      case 'close':
        _fail('连接已关闭');
    }
  }

  void _fail(String msg) {
    if (!mounted) return;
    setState(() {
      if (_state != _ConnState.connected || msg != '连接已关闭') _error = msg;
      _state = _ConnState.error;
    });
  }

  @override
  void dispose() {
    _sub?.cancel();
    _channel?.sink.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text('终端 · ${widget.ssh.host}:${widget.ssh.port}'),
        actions: [
          if (_state != _ConnState.connecting)
            IconButton(
              icon: const Icon(Icons.refresh),
              tooltip: '重连',
              onPressed: () {
                _channel?.sink.close();
                _connect();
              },
            ),
        ],
      ),
      body: Column(
        children: [
          if (_state == _ConnState.connecting)
            const LinearProgressIndicator(),
          if (_state == _ConnState.error)
            Padding(
              padding: const EdgeInsets.all(8),
              child: Row(children: [
                const Icon(Icons.error_outline, color: Colors.red, size: 18),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(_error,
                      style:
                          const TextStyle(color: Colors.red, fontSize: 12)),
                ),
              ]),
            ),
          Expanded(
            child: SafeArea(
              child: TerminalView(_terminal),
            ),
          ),
        ],
      ),
    );
  }
}
