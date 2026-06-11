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
  - icon: 📦
    title: 统一资源视图
    details: 通过 Inventory YAML 和 SQLite 管理 Agent、计算实例、域名、证书、服务、网关和存储
  - icon: 🌍
    title: Agent 主动连接
    details: Agent 通过 WebSocket 主动连接 Server，适合 NAT 后节点和跨地域环境
  - icon: 📊
    title: 系统指标
    details: Agent 心跳上报 CPU、内存、磁盘、网络和系统信息，Server 保存历史和快照
  - icon: 🔐
    title: 认证与审计
    details: 支持管理员初始化、JWT、TOTP、密码重置、审计日志和 Agent secret
  - icon: 🖥️
    title: 远程连接
    details: 终端、VNC 和桌面连接使用短期 ticket，经 Server 和 Agent 转发到目标服务
  - icon: 🧭
    title: 清晰边界
    details: Server 负责控制面和持久化，Agent 负责节点侧采集、代理和执行
---
