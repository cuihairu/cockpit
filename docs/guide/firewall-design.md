# 防火墙观测设计（iptables/nftables 只读）

## 背景

防火墙是安全面最后一块只读观测空白。个人 homelab 控制台「看一圈防火墙状态」是高频巡检动作：默认策略是否被人改过、有没有意外放行的端口、cockpit 自己依赖的入口是否还在。与 SMART/NAS/Overlay 同族——先观测，再告警，写操作远期再议（见 decision-log D-2026-10-07-3）。

Cockpit 现状：

- agent 已有成熟的 per-agent 观测骨架可整体复刻：capability 探测（`internal/agent/detector/`）、白名单 provider（`internal/agent/rpc/smart_provider.go` 为范本）、server 纯转发端点（`internal/server/api_smart.go` 为范本）；
- 全仓 grep 无任何 iptables/nftables 写路径——cockpit 至今不创建、不修改防火墙规则，本设计只观测。

本任务给 M1 补上数据面：**规则集快照只读观测 + Web 页只读三态**。不含巡检循环与告警（后续批次评估）、不含任何写操作。

## 决策

- **D1 新建 firewall capability，不挂靠现有 detector**。现有 capability（hardware-monitor/openwrt/overlay 等）没有承载防火墙工具探测的语义位；`internal/agent/detector/firewall.go` 新建：`LookPath` 探测 `nft` 与 `iptables`，能找到任一即上报 `firewall` capability，metadata 带工具清单（如 `{"nft": true, "iptables": true}`）。两者皆缺（典型如 Windows/裸容器）→ 不上报 capability，页面走「无防火墙工具」空态，不报错（smart D1 同则）。
- **D2 后端判定以实测为准，不猜发行版**。provider 运行 `nft list ruleset` 成功 → backend=`nftables`；失败或 nft 不存在但 `iptables-save` 成功 → backend=`iptables`。`iptables --version` 里的 `nf_tables`/`legacy` 字样只作 metadata 展示，不参与选择逻辑。
- **D3 RPC 单方法 `firewall.status`**，只读、无入参、校验面为零（smart D2/overlay 同风格）。
- **D4 数据面命令与超时**：
  - nftables：`nft -j list ruleset`（JSON 一次拿全量）；
  - iptables：`iptables-save`（全量文本，逐行解析）；
  - 超时：单命令 10s，整次 RPC 30s（smart D3 同量级）；
  - 输出体积上限 4MB：超限截断并置 `truncated: true`，页面明确提示「规则集过大，仅显示前 4MB」——极端机型的全量 ruleset 不能把 RPC 和前端内存打爆；
  - 读数命令失败 → `available=false` + `error` 原因，不像 SMART 逐盘粒度——防火墙是单集合，没有「部分成功」。
- **D5 字段白名单，不透传原始 JSON**。nft `-j` 的完整对象树很大且含 counters 噪声，provider 解析为扁平规则列表：每条 `handle`（nft）/行号（iptables）、`text`（人读摘要）、`packets`/`bytes`（有则带）。iptables-save 逐行切 `*table`/`:chain policy`/规则行三元组。绝不把原始输出整段塞给前端（smart D4 同则）。
- **D6 摘要统计与默认策略单列**。按 `family×table×chain` 聚合规则数；input/forward/output 等基础链的 `policy`（accept/drop）单列提升为页面徽标——「默认策略是不是 accept」是巡检第一眼。
- **D7 cockpit 名下标注：预留识别、不在 M1 造空功能**。cockpit 现在不创建任何防火墙规则（全仓无写路径），M1 页面无标注可打。预留机制：将来 cockpit 引入规则时统一带注释标记（iptables `-m comment --comment "cockpit:xxx"`、nft `comment "cockpit:xxx"`），provider 识别 `cockpit:` 前缀置 `ownedByCockpit=true`，前端高亮。机制先写进本决策，代码等第一条规则真实存在时再启用。
- **D8 权限现实**：`nft list ruleset`/`iptables-save` 需 root（CAP_NET_ADMIN/net_admin）。cockpit agent 默认以 root systemd 服务运行；非 root 部署 → `available=false` + 权限错误说明，页面提示（smart D6 同则）。OpenWrt 节点通常默认 root 且自带 nft（fw4），firewall capability 与 openwrt capability 并存不冲突，防火墙页对 OpenWrt 同样可用。
- **D9 REST 纯转发不落库**：`GET /api/agents/{id}/firewall/status` 透传 RPC 结果，浏览类端点不记审计（smart D7 同则）。M1 无全局配置（不做巡检间隔），故无 `/api/firewall/config`。
- **D10 权限位**：rbac `agentSubResources` 增 `"firewall/": "firewall"`（与 `"smart/": "smart"` 同构）；`firewall` 为单一权限档（M1 只读，无需 read/write 分档）。
- **D11 前端 `/firewall` 页（只读三态）**：
  1. agent 离线或无 firewall capability → 空态引导（「该主机未检测到 nftables/iptables 工具」）；
  2. `available=false`（工具在但读数失败，典型为无权限）→ 说明态（`error` 原因 + 非 root 部署提示）；
  3. 有数据 → 总览条（backend 类型、规则总数、input/forward/output 默认策略徽标）+ Collapse 按 family×table→chain 展开规则明细（`text` + counters）。
  菜单名「防火墙」，`SafetyOutlined`，置于「磁盘健康」之后。`truncated=true` 时页顶 Alert。
- **D12 不碰的东西**：只观测，无任何规则增删改路径；云安全组（AWS SG 等）不在范围；「写操作（规则增删）」明确留给后续批次且需真机验收纪律（decision-log D-2026-10-07-3 备选案已否决直接做写）。

## 数据结构

`firewall.status` 返回：

```json
{
  "available": true,
  "backend": "nftables",
  "backendVersion": "nft 1.0.9",
  "iptablesVariant": "",
  "totalRules": 42,
  "tables": [
    {
      "family": "inet",
      "name": "filter",
      "chains": [
        {
          "name": "input",
          "policy": "accept",
          "rules": [
            {
              "handle": 12,
              "text": "tcp dport 9000 accept",
              "packets": 1234,
              "bytes": 567890,
              "ownedByCockpit": false
            }
          ]
        }
      ]
    }
  ],
  "truncated": false,
  "error": ""
}
```

- iptables 后端：`backend="iptables"`，`iptablesVariant` 为 `nf_tables`/`legacy`（来自 `iptables --version`，仅展示）；tables 无 family 概念，`family=""`；规则 `handle` 省略，`text` 为规则行原文。
- `available=false` 时 tables 为空、`error` 带原因；工具全缺时 `backend=""`。
- `totalRules` 供总览条与后续告警批次做基线比对。

## 实现切分（M1，本任务）

1. **docs**：本文档；
2. **agent**：`detector/firewall.go`（capability 探测）+ `rpc/firewall_parse.go`（nft JSON / iptables-save 文本纯函数解析）+ `rpc/firewall_provider.go`（`FirewallProvider`，Commander 可注入）+ `providers.go` 注册 + 测试；
3. **server**：`api_firewall.go`（status 纯转发）+ 路由注册 + rbac 前缀 + 测试；
4. **web**：类型 + api 方法 + `/firewall` 页 + 菜单/路由 + `tsc`/build 通过；
5. **mobile**：不在 M1（firewall 观测页留后续批次，与 M3 三页同模式）。

## 测试要点

- 解析器：nft JSON（含 counters/无 counters、comment 前缀识别、非法 JSON 不 panic）；iptables-save（多表、`:chain POLICY` 行、空表）；超限截断逻辑；
- provider：fake Commander 注入；nft 缺失走 iptables 分支；两命令全失败 → `available=false`；`--version` 解析 legacy/nf_tables；
- detector：nft/iptables 各自存在与全缺四种组合的 capability/metadata 断言；
- server：离线 503、agent error 502 透传、rbac `firewall/` 前缀映射；
- 前端：三态渲染（无 capability/available=false/正常数据）、truncated Alert、`tsc --noEmit` + vite build。
