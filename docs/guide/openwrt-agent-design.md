# OpenWrt Agent 打包设计

## 背景与目标

cockpit-agent 面向个人混合基础设施，OpenWrt 路由器是其中最常见的
真机节点。此前 nightly 构建只覆盖 linux-amd64 / windows-amd64 /
darwin-arm64，OpenWrt 设备（musl + mipsel/mips/arm 等）无可用产物。

目标：CI 一键产出 OpenWrt 各主流架构的 **.ipk 安装包**（opkg 即装即
用，procd 托管）与 tar.gz 手动包。

## 前置：拆包根除 CGO 依赖

**根因**：agent 二进制曾把 sqlite 链进来。Go 的 import 是包级的——
`cmd/cockpit-agent` 只用 start 命令，但该命令原放在 `internal/cli`，
而 cli 包里的 init/status/sync 依赖 storage（gorm + mattn/go-sqlite3，
Cgo 绑定），于是 agent 全量继承。agent 运行时根本不开数据库。

**修复**：按领域而非技术分层组织包——

- `internal/cli/agent.go` → `internal/agent/startcmd.go`：agent 域
  命令归 agent 自己的 namespace（`StartCmd` / `StartUsage`）
- `internal/proxy/manager.go` → `internal/proxy/mgr/manager.go`：
  proxy 数据面（handler）与管理面（mgr）分家，manager 对 storage 的
  依赖不再污染 proxy 包

拆包后 agent 依赖闭包（`go list -deps ./cmd/cockpit-agent`）仅剩：

```
internal/agent{,/detector,/provider,/rdp,/rpc}
internal/{docker,openwrt,protocol,proxy,pve}
```

无 storage、无 sqlite、无 gorm。**纯 Go 交叉编译一条命令出静态包**，
不再需要 zig 或各架构 OpenWrt SDK：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
  go build -trimpath -ldflags '-s -w' -o cockpit-agent ./cmd/cockpit-agent
```

（已验证：产出 9.7MB ELF 32-bit LSB MIPS, statically linked, stripped，
musl/glibc 通吃。）

## 架构矩阵

| GOARCH | GOARM/GOMIPS | OpenWrt arch 名 | 覆盖设备 |
|---|---|---|---|
| mipsle | softfloat | mipsel | mt7621/mt7628 等主流路由 |
| mips | softfloat | mips | ar71xx 等传统设备 |
| arm64 | — | aarch64_generic | ARM 路由器/树莓派 |
| arm | GOARM=7 | arm_cortex-a7_neon-vfpv4 | ipq40xx/mt7623 等 32 位 ARM |
| amd64 | — | x86_64 | x86 软路由 |

mips/mipsle 设定 `GOMIPS=softfloat`：主流 OpenWrt mips 设备
（mt7621 等）为 soft-float。设备架构自查：`opkg print-architecture`
或 `uname -m`。

## ipk 包结构

ipk 为 ar 归档（debian-binary + control.tar.gz + data.tar.gz），
`ar` 手搓即可产出。内容：

```
/usr/bin/cockpit-agent
/etc/init.d/cockpit-agent     # procd 服务脚本
/etc/config/cockpit-agent     # UCI 配置（conffiles，升级保留）
```

- `control`：Package/Version/Architecture/Depends（无）/Provides。
- `conffiles`：/etc/config/cockpit-agent（升级不覆盖用户配置）。
- procd 脚本从 UCI 读参传给 `cockpit-agent start`：

```sh
config_get server 等字段 → procd_set_param command \
  /usr/bin/cockpit-agent start -server "$server" -secret "$secret" ...
procd_set_param respawn / procd_set_param stdout/stderr（日志进 logd）
```

## UCI 配置面

agent 启动参数与 flags 一一映射（无独立配置文件，UCI 为唯一真源）：

```
config agent 'main'
    option server  'wss://cockpit.example.com/ws'
    option secret  '...'
    option id      ''      # 可选，默认自动生成
    option region  ''
    option zone    ''
    option labels  ''      # key1=v1,key2=[a,b]
```

## CI 集成

nightly.yml 新增 `build-openwrt` job（ubuntu-latest）：

1. 矩阵交叉编译 5 架构（CGO_ENABLED=0，Go 原生工具链）
2. `scripts/package-ipk.sh` 组装 ipk 与 tar.gz
3. upload-artifact（agent-openwrt-\<arch\>，含两种格式）

手动触发即可用（workflow_dispatch 已有），不依赖 nightly 排程。

## 安装与升级

```sh
opkg install cockpit-agent_*_mipsel.ipk
uci set cockpit-agent.main.server='wss://...'
uci set cockpit-agent.main.secret='...'
/etc/init.d/cockpit-agent enable && /etc/init.d/cockpit-agent start
```

升级：`opkg install 新包`，UCI 配置保留（conffiles），procd 自动重启。

## 非目标

- LUCI 界面（个人使用，uci 足够）
- opkg 软件源仓库托管（artifact 分发即可，后续需要再做）
- macvtap/overlay 等网络能力在 OpenWrt 的适配验证（agent 核心监控
  功能不依赖它们；运行异常再按需处理）
