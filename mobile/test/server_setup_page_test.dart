import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/pages/server_setup_page.dart';
import 'package:cockpit_mobile/state/auth.dart';
import 'package:cockpit_mobile/state/settings.dart';

/// 可写 settings：记录 setServerUrl / setAllowSelfSigned 调用，可注入抛错。
class _WritableSettings extends SettingsNotifier {
  _WritableSettings(
      {this.url = '', this.selfSigned = true, this.throwOnSave = false});

  final String url;
  final bool selfSigned;
  final bool throwOnSave;
  String? savedUrl;
  bool? savedSelfSigned;

  @override
  SettingsState build() =>
      SettingsState(serverUrl: url, allowSelfSigned: selfSigned, loaded: true);

  @override
  Future<void> setServerUrl(String u) async {
    if (throwOnSave) throw StateError('disk full');
    savedUrl = u;
  }

  @override
  Future<void> setAllowSelfSigned(bool v) async => savedSelfSigned = v;
}

class _RecordingAuth extends AuthNotifier {
  int bootstraps = 0;

  @override
  AuthState build() => const AuthUnconfigured();

  @override
  Future<void> bootstrap() async => bootstraps++;
}

Future<void> _pump(WidgetTester tester, SettingsNotifier settings,
    {AuthNotifier? auth}) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      settingsProvider.overrideWith(() => settings),
      authProvider.overrideWith(() => auth ?? _RecordingAuth()),
      // 引导页自身不发 API；invalidate 重建也无消费者
      apiProvider.overrideWith((ref) async => throw StateError('unused')),
    ],
    child: const MaterialApp(home: ServerSetupPage()),
  ));
}

/// 在真 async 区起一个 /health 恒 200 的本地 server。
/// flutter_test 会装 HttpOverrides 拦下 HttpClient（一律 400），
/// 真实回环请求必须先还原 overrides；bind/listen 也要留在真 zone。
Future<HttpServer> _startServer(WidgetTester tester) async {
  final server = await tester.runAsync(() async {
    final saved = HttpOverrides.current;
    HttpOverrides.global = null;
    try {
      final s = await HttpServer.bind('127.0.0.1', 0);
      s.listen((req) async {
        req.response.statusCode = 200;
        await req.response.close();
      });
      return s;
    } finally {
      HttpOverrides.global = saved as HttpOverrides?;
    }
  });
  return server!;
}

/// 在真 async 区点「测试并保存」并等真网络收尾：done 非 null 时轮询其结果。
Future<void> _saveAndWait(
    WidgetTester tester, Object? Function()? done) async {
  await tester.runAsync(() async {
    final saved = HttpOverrides.current;
    HttpOverrides.global = null;
    try {
      final btn = tester.widget<FilledButton>(find.byType(FilledButton));
      btn.onPressed!();
      if (done == null) {
        await Future<void>.delayed(const Duration(milliseconds: 600));
        return;
      }
      for (var i = 0; i < 50; i++) {
        await Future<void>.delayed(const Duration(milliseconds: 100));
        if (done() != null) return;
      }
    } finally {
      HttpOverrides.global = saved as HttpOverrides?;
    }
  });
}

void main() {
  testWidgets('引导页：空地址提示', (tester) async {
    await _pump(tester, _WritableSettings());
    await tester.tap(find.text('测试并保存'));
    await tester.pumpAndSettle();
    expect(find.text('请输入 Server 地址'), findsOneWidget);
  });

  testWidgets('引导页：连通成功 → 去尾斜杠落盘 + bootstrap（自签开）',
      (tester) async {
    final server = await _startServer(tester);
    final settings = _WritableSettings(url: '', selfSigned: true);
    final auth = _RecordingAuth();
    try {
      await _pump(tester, settings, auth: auth);
      await tester.enterText(find.widgetWithText(TextField, 'Server 地址'),
          'http://127.0.0.1:${server.port}/');
      // 真网络在 fake-async 里不会推进：把触发与等待都放进真 async 区，
      // 让 dio 在真 zone 上建立 socket。
      await _saveAndWait(tester, () => settings.savedUrl);
      await tester.pumpAndSettle();

      expect(settings.savedUrl, 'http://127.0.0.1:${server.port}');
      expect(auth.bootstraps, 1);
      expect(find.text('请输入 Server 地址'), findsNothing);
    } finally {
      await tester.runAsync(() => server.close());
    }
  });

  testWidgets('引导页：连通成功（自签关，走严格 HttpClient 分支）',
      (tester) async {
    final server = await _startServer(tester);
    final settings = _WritableSettings(url: '', selfSigned: false);
    try {
      await _pump(tester, settings);
      await tester.enterText(find.widgetWithText(TextField, 'Server 地址'),
          'http://127.0.0.1:${server.port}');
      await _saveAndWait(tester, () => settings.savedUrl);
      await tester.pumpAndSettle();

      expect(settings.savedUrl, 'http://127.0.0.1:${server.port}');
    } finally {
      await tester.runAsync(() => server.close());
    }
  });

  testWidgets('引导页：端口不通 → 连接失败', (tester) async {
    // 占一个端口再关掉，保证连接被拒
    final port = await tester.runAsync(() async {
      final saved = HttpOverrides.current;
      HttpOverrides.global = null;
      try {
        final holder = await HttpServer.bind('127.0.0.1', 0);
        final p = holder.port;
        await holder.close();
        return p;
      } finally {
        HttpOverrides.global = saved as HttpOverrides?;
      }
    });
    final settings = _WritableSettings(url: '');
    await _pump(tester, settings);
    await tester.enterText(find.widgetWithText(TextField, 'Server 地址'),
        'http://127.0.0.1:$port');
    // 连接被拒在真 event loop 上完成，固定等一拍
    await _saveAndWait(tester, () => null);
    await tester.pumpAndSettle();

    expect(find.textContaining('连接失败'), findsOneWidget);
  });

  testWidgets('引导页：落盘抛错 → 通用 catch 提示', (tester) async {
    final server = await _startServer(tester);
    final settings = _WritableSettings(url: '', throwOnSave: true);
    try {
      await _pump(tester, settings);
      await tester.enterText(find.widgetWithText(TextField, 'Server 地址'),
          'http://127.0.0.1:${server.port}');
      // setServerUrl 抛错后 _testing 复位，固定等一拍
      await _saveAndWait(tester, () => null);
      await tester.pumpAndSettle();

      expect(find.textContaining('连接失败'), findsOneWidget);
      expect(find.textContaining('disk full'), findsOneWidget);
    } finally {
      await tester.runAsync(() => server.close());
    }
  });

  testWidgets('引导页：自签开关切换落盘', (tester) async {
    final settings = _WritableSettings(selfSigned: true);
    await _pump(tester, settings);
    await tester.tap(find.byType(Switch).first);
    await tester.pumpAndSettle();
    expect(settings.savedSelfSigned, isFalse);
  });
}
