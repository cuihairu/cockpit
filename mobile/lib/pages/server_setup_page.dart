import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:dio/io.dart';
import 'dart:io';

import '../state/auth.dart';
import '../state/settings.dart';

/// 引导页：配置 server 地址并测试连通（GET /health，无需认证）。
class ServerSetupPage extends ConsumerStatefulWidget {
  const ServerSetupPage({super.key});

  @override
  ConsumerState<ServerSetupPage> createState() => _ServerSetupPageState();
}

class _ServerSetupPageState extends ConsumerState<ServerSetupPage> {
  final _controller = TextEditingController();
  bool _testing = false;
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _test() async {
    final url = _controller.text.trim();
    if (url.isEmpty) {
      setState(() => _error = '请输入 Server 地址');
      return;
    }
    setState(() {
      _testing = true;
      _error = null;
    });
    try {
      final s = ref.read(settingsProvider);
      final dio = Dio(BaseOptions(
        baseUrl: url.endsWith('/') ? url.substring(0, url.length - 1) : url,
        connectTimeout: const Duration(seconds: 6),
        receiveTimeout: const Duration(seconds: 10),
      ));
      final adapter = dio.httpClientAdapter as IOHttpClientAdapter;
      adapter.createHttpClient = () {
        final http = HttpClient();
        if (s.allowSelfSigned) {
          http.badCertificateCallback = (_, _, _) => true;
        }
        return http;
      };
      await dio.get<String>('/health');
      await ref.read(settingsProvider.notifier).setServerUrl(dio.options.baseUrl);
      // apiProvider 依赖 settingsProvider，地址落盘后失效重建，路由随之切到登录页。
      ref.invalidate(apiProvider);
      await ref.read(authProvider.notifier).bootstrap();
    } on DioException catch (e) {
      setState(() => _error = '连接失败：${e.message ?? e.type.name}');
    } catch (e) {
      setState(() => _error = '连接失败：$e');
    } finally {
      if (mounted) setState(() => _testing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final selfSigned = ref.watch(
        settingsProvider.select((s) => s.allowSelfSigned));
    return Scaffold(
      appBar: AppBar(title: const Text('连接 Cockpit Server')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          TextField(
            controller: _controller,
            keyboardType: TextInputType.url,
            decoration: const InputDecoration(
              labelText: 'Server 地址',
              hintText: 'https://cockpit.example.com',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          SwitchListTile(
            title: const Text('允许自签证书'),
            subtitle: const Text('存在中间人风险，仅在可信内网开启'),
            value: selfSigned,
            onChanged: (v) =>
                ref.read(settingsProvider.notifier).setAllowSelfSigned(v),
          ),
          const SizedBox(height: 12),
          FilledButton(
            onPressed: _testing ? null : _test,
            child: _testing
                ? const SizedBox(
                    width: 18, height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : const Text('测试并保存'),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!,
                style: TextStyle(
                    color: Theme.of(context).colorScheme.error)),
          ],
        ],
      ),
    );
  }
}
