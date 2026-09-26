---
layout: home

hero:
  name: Cockpit
  text: 个人混合基础设施控制台
  tagline: Server + Agent 的轻量资源视图、监控与远程连接入口
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/getting-started
    - theme: alt
      text: 架构与边界
      link: /guide/architecture

features:
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="3" y="7" width="18" height="13" rx="2"/><path d="M3 11h18"/><path d="M9 7V5a3 3 0 0 1 6 0v2"/></svg>'
    title: 统一资源视图
    details: 通过 Inventory YAML 和 SQLite 管理 Agent、计算实例、域名、证书、服务、网关和存储
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>'
    title: Agent 主动连接
    details: Agent 通过 WebSocket 主动连接 Server，适合 NAT 后节点和跨地域环境
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><path d="M4 20V10"/><path d="M10 20V4"/><path d="M16 20v-8"/><path d="M22 20H2"/></svg>'
    title: 系统指标
    details: Agent 心跳上报 CPU、内存、磁盘、网络和系统信息，Server 保存历史和快照
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/><circle cx="12" cy="15.5" r="1.5"/></svg>'
    title: 认证与审计
    details: 支持管理员初始化、JWT、TOTP、密码重置、审计日志和 Agent secret
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8"/><path d="M12 16v4"/><path d="m8 8 2.5 2L8 12"/><path d="M13 12h4"/></svg>'
    title: 远程连接
    details: 终端、VNC 和桌面连接使用短期 ticket，经 Server 和 Agent 转发到目标服务
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/><path d="M6 9h3"/><path d="M15 15h3"/></svg>'
    title: 清晰边界
    details: Server 负责控制面和持久化，Agent 负责节点侧采集、代理和执行
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4"/><path d="M10 8l4 4-4 4"/><path d="M14 12H3"/></svg>'
    title: Agent 出口
    details: 明确 Agent 作为内网访问出口的能力边界，以及与完整 SD-WAN 的差距
---
