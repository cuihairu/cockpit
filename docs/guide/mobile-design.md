# 移动端方案：Flutter（iOS + Android）

> 2026-09-22。用户指定技术栈 Flutter，双端覆盖。本文档定义选型依据、与既有
> server 的边界、M1 最小可用范围与后续里程碑。

## 背景

Cockpit web UI 是桌面优先（antd 数据密集表格、多栏工作台），手机浏览器体验差。
而个人基础设施控制台最典型的移动场景恰是：

- **在外面看状态**：agent 在线吗、告警了吗、证书/域名快到期了吗；
- **收告警**：磁盘 SMART 异常、agent 掉线、备份失败要能推到手机；
- **少量关键操作**：重启一个容器/服务、确认一条告警，而不是完整运维。

移动端功能面天然是 web 的窄子集，方案设计围绕「薄客户端 + 复用一切既有设施」。

## 决策

- **D1 技术选型 Flutter**（用户指定）。备选对比：PWA/响应式改造 web 成本最低，
  但 antd 表格小屏体验差、无推送通道、无原生手感，作为移动方案不达标；React
  Native 与 web 共享 TS 生态，但个人项目维护第二前端的成本同样存在，且终端模拟
  （xterm.dart）等原生组件 Flutter 侧更成熟。Flutter 代价是引入第三语言栈
  （Go / TS / Dart），以 M1 功能面窄（只读为主 + 少量操作）换取长期维护面可控。

- **D2 不建独立 BFF，直连既有 `/api/*`**：web 端 `services/api.ts` 已收敛 173 个
  端点调用，认证（JWT + TOTP 双步）、agent、告警、Docker、审计全部现成。移动端
  只做薄客户端；server 侧仅在确有聚合需求时补端点（M1 预计零新增）。API 对齐
  纪律同 web：**字段以 Go model json tag 为准**，不从记忆或互相抄。

- **D3 告警推送复用 ntfy 渠道，不接 FCM/APNs**：server 通知已有
  herald / ntfy / webhook / telegram 四渠道（config.go），其中 ntfy 官方
  Android/iOS app 就是现成的推送终端——server 配一条 ntfy 渠道即得手机推送，
  省掉 FCM/APNs 的账号、签名与后端管道。App 内另做告警列表轮询兜底。
  自托管 ntfy 的 iOS 即时送达有限制（后台轮询间隔），文档记录、不阻塞。

- **D4 认证完全复用**：`POST /api/auth/login` → `requires_totp=true` 时以
  `tmp_token` 走 `/api/auth/totp/verify` 换正式 JWT（web `LoginResponse` 同款
  双步流程）；token 存 `flutter_secure_storage`（iOS Keychain / Android
  Keystore），绝不落 SharedPreferences；`/api/auth/refresh` 续期由 dio 拦截器
  自动完成（401 → refresh → 重放，失败登出回登录页）。

- **D5 自签 HTTPS 显式开关**：server 常部署在内网或自签证书后。默认严格校验；
  设置页提供「允许自签证书」开关（`badCertificateCallback` 放行），开启时红字
  提示中间人风险。不做证书导入（个人场景过度设计）。

- **D6 状态管理 Riverpod + 网络 dio**：Riverpod 编译期安全、社区主流；dio 的
  拦截器机制直接承载 D4 的刷新重放与 D5 的证书开关。数据轮询为主（M1 不接
  WebSocket 指标流），仪表盘场景轮询省电且实现简单。

- **D7 终端 / 桌面后置**：终端 M2 用 xterm.dart + 既有 remote ticket WebSocket；
  VNC/桌面观看在 Flutter 侧生态弱（无 noVNC 等价物），列为探索项不承诺。

- **D8 CI 独立 `mobile.yml`**：`paths: mobile/**` 触发，`flutter analyze` +
  `flutter test` + APK 构建（`subosito/flutter-action`）。iOS 构建不进 CI
  （需 macOS runner + 签名账号），本地/真机阶段再议。

## 里程碑

**M1 最小可用**（手机场景核心闭环）：

| 页面 | 复用端点（对照 `web/src/services/api.ts`） |
| --- | --- |
| 引导：server URL 配置 + 连接测试 | `GET /health` |
| 登录（含 TOTP 双步） | `/api/auth/login`、`/api/auth/totp/verify` |
| 仪表盘：agent 在线/离线、未读告警、证书域名临期 | `/api/agents`、`/api/alerts`、资源列表 |
| Agent 列表 + 详情指标 | `/api/agents`、metrics 端点 |
| Docker 容器列表 + start/stop/restart | Docker 容器端点 |
| 告警列表 + 全部已读 | `/api/alerts`、`PUT /api/alerts/read-all` |
| 审计列表 | `/api/audit` |
| 设置：server URL、自签开关、退出 | — |

深色主题跟随系统；手机竖屏单栏布局。

**M2**（2026-09-22 完成）：资源页三 tab（域名/证书临期、备份任务状态与手动触发）、
Cron 只读视图（外部条目原文展示，写操作留桌面端）、文件浏览（只读：目录导航 +
条目详情）、SSH 终端（tickets 票据 + WS 子协议 + xterm.dart）、生物识别锁
（local_auth 启动门）。

与原计划的偏差：反代（nginx/traefik）视图**未做**，顺延 M3 评估；备份视图
落位为资源页第三 tab 而非独立页；Cron 与文件在移动端明确**只读**——增删改
类操作留桌面端，与「手机看、桌面改」的定位一致。

**M3**：SMART/NAS/组网观测只读视图、Stacks 部署操作、VNC 探索项。

## 目录结构

```
mobile/                  # Flutter 工程，与 web/ 平行
  lib/
    main.dart
    api/                 # dio client、拦截器、端点封装
    models/              # 手写与 Go json tag 对齐的模型
    state/               # Riverpod providers（auth、agents、alerts…）
    pages/               # 每页一目录，与 web pages 命名对齐
    widgets/
  test/                  # 单测：models 对齐、拦截器、页面 widget test
```

## 落地前提与验收

- 本机需安装 Flutter SDK（stable）。本机为 Linux：**Android APK 可本地构建；
  iOS 无法本地构建**（需 macOS + Xcode），iOS 侧交付形态为源码工程 + 真机验收
  项挂起，待有 Mac 或 CI macOS runner 再闭环。
- 自动化验收：`flutter analyze` 0 问题、`flutter test` 全绿、`mobile.yml` CI 绿。
- 真机验收（补入 `docs/guide/acceptance-checklist.md` 移动端一节）：
  - Android 真机安装 APK：登录 → TOTP → 仪表盘数据与 server 一致；
  - 断网/错误 server URL 的错误呈现；token 过期自动续期无感；
  - ntfy 渠道推送送达手机（server 侧触发告警）；
  - 自签开关关闭时自签 server 连接被拒、开启后可用。
