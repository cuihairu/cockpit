# 参考项目对比与借鉴

本文记录 Cockpit 与同类开源项目的对比分析，用于功能规划和设计决策。基于 2026-09 的公开资料整理，链接见文末来源。

Cockpit 的定位是**个人混合基础设施控制台**，四条主线：

1. Git-first CMDB（inventory YAML → SQLite）
2. Agent 主动出站 WebSocket（NAT 友好）
3. 运行态监控 + 告警 + 健康探测
4. Web 控制台（终端/桌面/VNC 远控 + Docker 管理）

按能力域选取 7 个对标项目。每个项目给出优缺点，并区分**可参考**（直接照做的具体做法）与**可借鉴**（吸收的设计理念）。

## 监控域：Beszel

[henrygd/beszel](https://github.com/henrygd/beszel) — MIT 协议，Go 编写，hub + agent 架构，与 Cockpit 的 Server + Agent 同构。2026 年已是 homelab / 小型 VPS 集群监控的主流选择。

**优点**

- 极轻量：每个被监控服务器上 agent 仅约 23MB RAM
- 单二进制部署，分钟级跑起来
- Docker 容器级 stats、历史数据、告警、通知推送齐全
- PocketBase 底座，运维简单

**缺点**

- 只做监控：无 CMDB、无远控、无管理动作、无资源视图
- 数据模型固定，扩展性有限

**可借鉴**

- Agent 资源占用是硬指标：Cockpit Agent 应建立内存/CPU 基线测量，避免膨胀
- 历史指标的紧凑存储方案（Cockpit 用 SQLite，可研究其降采样策略）
- "开箱即用"的部署体验：一条命令跑起来，默认配置即可用

## 多机容器管理：Komodo

[Komodo (komo.do)](https://komo.do/docs/intro) — Core + Periphery 架构，与 Cockpit 的 Server + Agent 完全同构，是最值得研究的多机管理参照。

**优点**

- **Git-to-deploy 工作流**：compose.yml 存 Git 仓库，变更自动部署到目标服务器
- Build 管道：镜像构建与部署一体化
- "自动化程序"（Procedures）：把常用运维动作编排成可复用过程
- 单 Core 管理多 Docker 主机

**缺点**

- 依赖 MongoDB，对个人单机场景偏重
- 只管容器：不做 CMDB、远控、桌面接入

**可借鉴**

- 直接对标路线图 P1"应用部署"：Compose 栈从 Git 拉取 + 触发 up/down 的模式
- "自动化程序"概念：Cockpit 已有 RPC 链路 + probe，把"重启栈 + 清理镜像 + 健康探测"编排成可复用过程成本低

## 容器面板：Portainer / Dockge

[Portainer vs Dockge 对比](https://talos.tools/compare/portainer-vs-dockge)、[OneUptime 2026 对比](https://oneuptime.com/blog/post/2026-03-20-portainer-vs-dockge/view)

| | Portainer | Dockge |
| --- | --- | --- |
| 优点 | 多主机/Swarm/K8s、企业治理、生态成熟（约 50MB 内存） | compose-file-first 哲学、文件即真相、单机简单直观 |
| 缺点 | 商业化趋重、部分闭源 | 单机局限、多机能力弱 |

**可参考**

- Compose Stack 的 UI 交互蓝本：编辑器 + 状态总览 + up/down 按钮 + 日志入口一屏完成。Cockpit 做 P1 应用部署页时可直接参照此布局
- Dockge 的 compose-file-first：YAML 文件本身是配置源头，UI 是视图——与 Cockpit 的 Git-first 理念一致

## 远控网关：Apache Guacamole

[Apache Guacamole](https://guacamole.apache.org/) — clientless 远程桌面网关标杆，浏览器直接开 VNC/RDP/SSH。

**优点**

- guacd 协议翻译层把协议处理与 Web 层解耦，架构清晰
- 会话录制、剪贴板/文件传输通道
- 连接管理、双因素认证成熟

**缺点**

- Java 技术栈重，需要单独维护 guacd 进程
- 连接是静态配置的，与 Cockpit 的动态 inventory 理念相反

**可借鉴**

- **会话录制**：Cockpit 审计目前记录会话开始/结束，可增加终端输出录制作为审计补强
- 文件传输通道：路线图 P1"文件管理器"之外，远控侧的文件进出能力补充

## 访问安全：Teleport

Teleport（[与 Guacamole 的社区对比](https://www.reddit.com/r/selfhosted/comments/1im5ioy/how_to_connect_to_sshrdp_web_based_with_teleport/)）— 身份为中心的访问平台。

**优点**

- 证书短时效访问、RBAC、完整会话录制
- Access request 审批流：高危操作需审批放行
- 会话粒度审计明细

**缺点**

- 企业级复杂度与资源占用，个人场景过重

**可参考**

- Cockpit 的短时效 ticket + `Sec-WebSocket-Protocol` 认证链路与其思路一致，**设计已验证正确**

**可借鉴**

- 按会话粒度的审计明细（命令级记录是远期方向）
- `allow_arbitrary_target` 这类高危开关可升级为"单次审批放行"模式

## CMDB：NetBox

[NetBox Labs：source of truth 理念](https://netboxlabs.com/blog/the-long-war-against-configuration-chaos/)、[NetBox GitOps 集成](https://netodata.io/netbox-integration-connecting-dcim-ipam-with-enterprise-infrastructure/) — DCIM/IPAM 领域最成熟的 declarative source of truth。

**优点**

- "声明式真相源"范式最成熟：API-first、期望态（desired）与实际态（actual）分离
- Terraform / Ansible 的 GitOps 集成生态完整
- 资源建模精细：标签、自定义字段、资源关系

**缺点**

- Python/Django 技术栈，DCIM/IPAM 领域模型重，不适合个人单机轻量场景

**可借鉴**

- inventory schema 字段建模可对照其 resource 模型补齐（标签、资源关系、自定义字段）
- **desired state vs actual state 分离 + drift（漂移）视图**：Cockpit 的 Syncer 已是此模式，可在 API/UI 暴露"inventory 声明与 Agent 实报不一致"的高亮视图——这是 Git-first CMDB 的差异化卖点

## 拨测：Uptime Kuma

[Uptime Kuma](https://uptimekuma.co/) / [louislam/uptime-kuma](https://github.com/louislam/uptime-kuma) — 自托管拨测监控标杆。

**优点**

- 20 秒级探测间隔，多协议（HTTP(s)/TCP/ping/DNS/关键字/Docker 容器）
- 90+ 通知渠道（ntfy/Telegram/Discord/Slack/邮件等）
- 公开状态页、心跳条式历史 UI

**缺点**

- Node.js 常驻资源偏高
- 只能从自身位置拨测，**无 agent 分布式探测**
- 无 CMDB 联动，监控目标需手工录入

**可借鉴**（对照 Cockpit 刚落地的 `internal/probe`）

- 探测间隔可配置（当前硬编码 5 分钟）
- 通知渠道集成：告警框架已有，补 ntfy/webhook/Telegram 输出
- 心跳条式状态历史 UI、公开状态页
- **Cockpit 的差异化优势**：probe 结果天然联动 inventory 资源、且可经 Agent 所在网络分布式探测——这是 Uptime Kuma 架构做不到的，应重点强化而非回避

## 汇总矩阵

| 项目 | 重叠域 | 可参考（抄做法） | 可借鉴（吸收理念） | 不值得学 |
| --- | --- | --- | --- | --- |
| Beszel | 监控/agent | 历史数据紧凑存储 | agent 轻量性硬指标、部署体验 | —— |
| Komodo | 容器/agent | —— | Git-to-deploy、自动化程序编排 | MongoDB 依赖 |
| Portainer/Dockge | Docker UI | Compose 栈页面交互 | compose-file-first | 企业治理层 |
| Guacamole | 远控 | —— | 会话录制、文件传输 | Java 栈、静态连接配置 |
| Teleport | 远控安全 | ticket 短时效（已验证） | 审批流、会话粒度审计 | 企业级复杂度 |
| NetBox | CMDB | —— | 期望态/实际态分离、drift 视图 | 领域模型重量 |
| Uptime Kuma | 拨测 | 心跳条 UI | 通知渠道、状态页 | 单点探测模式 |

## 对功能规划的结论

1. **P1"应用部署（Compose Stack）"首选启动**：同时借鉴 Komodo 的 Git-to-deploy 与 Dockge 的 compose-file-first；Cockpit 已有 `docker_provider` RPC 链路，投入产出比最高。
2. **probe 增强**：间隔可配置 + 通知渠道，成本低，让已落地的健康探测立刻产生实际价值。
3. **会话录制**（远期）：借鉴 Guacamole/Teleport，补强远控审计。
4. **drift 视图**（远期）：借鉴 NetBox，强化 Git-first CMDB 差异化。

## 来源

- [Beszel 官网](https://www.beszel.dev/) / [henrygd/beszel](https://github.com/henrygd/beszel)
- [7 款自托管监控工具实测（2026）](https://dev.to/vikasprogrammer/i-tested-7-self-hosted-monitoring-tools-on-a-3-vps-in-2026-heres-the-one-i-kept-aoa)
- [RamNode Beszel 指南](https://www.ramnode.com/guides/series/monitoring/beszel)
- [Komodo 文档](https://komo.do/docs/intro)
- [Komodo vs Portainer：Git-to-deploy](https://medium.com/@mariomarco08/komodo-vs-portainer-which-fits-a-git-to-deploy-workflow-715e8ef49bd3)
- [Bitdoze：五个 Docker 管理 UI 实测](https://www.bitdoze.com/portainer-alternatives/)
- [Portainer vs Dockge（OneUptime，2026）](https://oneuptime.com/blog/post/2026-03-20-portainer-vs-dockge/view)
- [Portainer vs Dockge（Talos.tools）](https://talos.tools/compare/portainer-vs-dockge)
- [Apache Guacamole 官网](https://guacamole.apache.org/)
- [Teleport/Guacamole 社区对比](https://www.reddit.com/r/selfhosted/comments/1im5ioy/how_to_connect_to_sshrdp_web_based_with_teleport/)
- [NetBox Labs：The Long War Against Configuration Chaos](https://netboxlabs.com/blog/the-long-war-against-configuration-chaos/)
- [NetBox GitOps 集成](https://netodata.io/netbox-integration-connecting-dcim-ipam-with-enterprise-infrastructure/)
- [Uptime Kuma 官网](https://uptimekuma.co/) / [louislam/uptime-kuma](https://github.com/louislam/uptime-kuma)
