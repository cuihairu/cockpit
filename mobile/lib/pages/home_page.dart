import 'package:flutter/material.dart';

import 'agents_page.dart';
import 'alerts_page.dart';
import 'dashboard_page.dart';
import 'resources_page.dart';
import 'settings_page.dart';

/// 登录后主框架：底部五 tab。
/// 返回键/手势语义（对齐 Material 规范）：非首页 tab → 回首页 tab；
/// 首页 tab → 退出 app（默认行为）。
class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  int _index = 0;

  @override
  Widget build(BuildContext context) {
    final pages = [
      const DashboardPage(),
      const AgentsPage(),
      const ResourcesPage(),
      const AlertsPage(),
      const SettingsPage(),
    ];
    return PopScope(
      canPop: _index == 0,
      onPopInvokedWithResult: (didPop, _) {
        if (didPop) return;
        // 非首页 tab：返回先回首页 tab，不退出
        setState(() => _index = 0);
      },
      child: Scaffold(
        body: IndexedStack(index: _index, children: pages),
        bottomNavigationBar: NavigationBar(
          selectedIndex: _index,
          onDestinationSelected: (i) => setState(() => _index = i),
          destinations: const [
            NavigationDestination(
                icon: Icon(Icons.dashboard_outlined),
                selectedIcon: Icon(Icons.dashboard),
                label: '仪表盘'),
            NavigationDestination(
                icon: Icon(Icons.dns_outlined),
                selectedIcon: Icon(Icons.dns),
                label: '主机'),
            NavigationDestination(
                icon: Icon(Icons.folder_outlined),
                selectedIcon: Icon(Icons.folder),
                label: '资源'),
            NavigationDestination(
                icon: Icon(Icons.notifications_outlined),
                selectedIcon: Icon(Icons.notifications),
                label: '告警'),
            NavigationDestination(
                icon: Icon(Icons.settings_outlined),
                selectedIcon: Icon(Icons.settings),
                label: '设置'),
          ],
        ),
      ),
    );
  }
}
