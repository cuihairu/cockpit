import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'pages/home_page.dart';
import 'pages/login_page.dart';
import 'pages/server_setup_page.dart';
import 'state/auth.dart';
import 'state/settings.dart';
import 'widgets/lock_gate.dart';

void main() {
  runApp(const ProviderScope(child: CockpitApp()));
}

class CockpitApp extends ConsumerStatefulWidget {
  const CockpitApp({super.key});

  @override
  ConsumerState<CockpitApp> createState() => _CockpitAppState();
}

class _CockpitAppState extends ConsumerState<CockpitApp> {
  @override
  void initState() {
    super.initState();
    // settings 从磁盘加载完成后做登录态恢复。
    ref.listenManual(settingsProvider, (prev, next) {
      if (next.loaded && (prev == null || !prev.loaded)) {
        ref.read(authProvider.notifier).bootstrap();
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final settings = ref.watch(settingsProvider);
    final api = ref.watch(apiProvider);
    final auth = ref.watch(authProvider);

    Widget page;
    if (!settings.loaded) {
      page = const Scaffold(
          body: Center(child: CircularProgressIndicator()));
    } else if (!settings.configured) {
      page = const ServerSetupPage();
    } else {
      page = api.when(
        loading: () => const Scaffold(
            body: Center(child: CircularProgressIndicator())),
        error: (e, _) => const ServerSetupPage(),
        data: (_) => switch (auth) {
          AuthTotpRequired() => const LoginPage(),
          Authenticated() =>
            // 生物识别锁只护已登录视图；登录流程本身已要求认证
            settings.biometricLock
                ? const LockGate(child: HomePage())
                : const HomePage(),
          _ => const LoginPage(),
        },
      );
    }

    return MaterialApp(
      title: 'Cockpit',
      theme: ThemeData(colorSchemeSeed: const Color(0xFF1E88E5)),
      darkTheme: ThemeData(
          brightness: Brightness.dark,
          colorSchemeSeed: const Color(0xFF1E88E5)),
      themeMode: ThemeMode.system,
      home: page,
    );
  }
}
