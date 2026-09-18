# NAS 系统对接设计

> 状态：M1 + M2-DSM 设计定稿。统一快照模型 + 多 Provider 架构：M1 通用 Linux
> NAS（agent 本地命令观测，零凭据零配置）打通全链路；M2 群晖 DSM / TrueNAS /
> OpenMediaVault 网络 API provider（agent 内网访问，OpenWrt/PVE 同款），DSM
> 已实现，TrueNAS/OMV 排队。

## D1 目标与统一模型

**目标**：把常见 NAS 的存储池、挂载容量、共享导出统一为一屏可见的观测面 +
异常告警。只做观测（M1），不做 NAS 配置变更写路径。

统一快照模型（所有 provider 映射到同一结构，前端与告警不感知后端差异）：

```go
type NasSnapshot struct {
    Available bool       // 该 provider 是否可用（工具缺失/API 不通时 false，不报错）
    Source    string     // linux | dsm | truenas | omv
    Pools     []NasPool  // 存储池 / RAID / 卷组
    Mounts    []NasMount // 本地文件系统挂载容量
    Shares    []NasShare // SMB / NFS 共享导出
}

type NasPool struct {
    Name     string   // md0, tank, vg0, volume_1 ...
    Kind     string   // mdadm | zfs | lvm | btrfs | dsm（厂商池以来源标识）
    State    string   // healthy | degraded | resync | failed | unknown
    TotalGB  float64
    UsedGB   float64  // ZFS/btrfs 可得；拿不到留 0
    Devices  []string
    Detail   string   // 原始状态行（resync 进度等）
    Host     string   // 来源设备：本地观测为空，网络 NAS 填 target 名（M2）
}

type NasMount struct {
    Device    string
    MountPath string
    FsType    string
    TotalGB   float64
    UsedGB    float64
    Host      string  // 同上
}

type NasShare struct {
    Protocol string // smb | nfs
    Name     string
    Path     string
    Comment  string
    Hosts    string // NFS 导出范围（如 192.168.1.0/24）
    Host     string // 同上
}
```

磁盘健康（SMART）不在这里重复——Disk 页已覆盖；NAS 页聚焦池/容量/共享。

## D2 Provider 架构与里程碑

`NasProvider` 实现三选一（按 capability/配置决定，agent 侧全部经 RPC 暴露）：

| Provider | 路线 | 数据源 | 里程碑 |
|---|---|---|---|
| linux | agent 本地命令 | /proc/mdstat、zpool、vgs、btrfs、df、testparm、exportfs | **M1** ✅ |
| dsm | agent→DSM HTTP API | SYNO.API 系列接口，Session 登录 | **M2** ✅ |
| truenas | agent→TrueNAS REST/WebSocket | /api/v2.0 pool/dataset/sharing | M2 排队 |
| omv | agent→OMV JSON-RPC | Login + Rpc | M2 排队 |

一个 agent 可观测多台网络 NAS（targets 数组），快照合并返回：来源设备标在
每条记录的 `Host` 字段（本地观测为空，前端主机列显示 `agent · 设备`），
`Source` 用逗号 join 参与源（`linux,dsm`）；单 target 失败只 log 降级不拖垮
快照（与单数据源失败同纪律）。

M2 网络 API 型的共同纪律（OpenWrt/PVE provider 同款）：

- agent 在内网主动发起 HTTP（agent 本就主动外连 server，内网横向访问成立）
- 凭据从 agent 环境变量 `COCKPIT_NAS_TARGETS`（JSON 数组）读取，
  **凭据不落库、不进 config.yaml、绝不出现在快照/日志/审计/错误消息**：

  ```json
  [
    {"name":"home-dsm","type":"dsm","addr":"https://192.168.1.10:5001",
     "username":"cockpit","password":"***","insecureTls":true}
  ]
  ```

  - `type` 不是已实现类型（现仅 `dsm`）的条目直接忽略
  - 解析失败/缺字段（name/addr/username/password）的条目丢弃并 log，不致命
- 自签证书默认 InsecureTLS（家用 NAS 常态），目标项可关（`insecureTls: false`）
- 快照映射到同一 NasSnapshot，Source 标来源

## D3 M1：linux provider 数据源（每源独立降级，单源失败不拖垮快照）

| 类别 | 命令 | 解析要点 |
|---|---|---|
| mdadm RAID | `/proc/mdstat` 读文件 | `md0 : active raid1 sda1[0] sdb1[2](F)` → (F) 标记 failed 盘；resync/recovery 进度行 → State=resync；`[U_]` 位图缺 U → degraded |
| ZFS 池 | `zpool list -H -p -o name,size,alloc,health` | health 列 ONLINE→healthy，其余原词小写；无 zpool 跳过 |
| LVM 卷组 | `vgs --noheadings --units b --nosuffix -o vg_name,vg_size,vg_free` | VG 容量与剩余；无 vgs 跳过 |
| btrfs | —（不池化） | 单盘 btrfs 容量经 df 挂载表可见，M1 不做池化解析 |
| 挂载容量 | `df -k -P`（GNU/busybox 通用） | source 以 `/` 开头且非 `//` 前缀（排除 tmpfs/overlay/网络挂载等伪与远端 FS）且 ≥1GB；同 (source,容量) 去重防 bind mount 噪声 |
| SMB 共享 | `testparm -s`（stderr 吐出配置） | 解析非 global `[share]` 段的 path/comment；无 samba 跳过 |
| NFS 导出 | `exportfs -v` | 行格式 `<path> <host>(opts)`；空输出 fallback `/etc/exports` 同规则；无 nfs-utils 跳过 |

- 池 State 判定：mdadm → 有 (F) 盘或位图 [U_] 缺 U = degraded、resync/recovery/check
  进度行 = resync、inactive/failed = failed、否则 healthy；ZFS → ONLINE=healthy、
  其余原词小写；LVM 无状态概念=unknown（不告警）
- `Available` = 任一子源有产出或任一工具存在；全部缺失（纯容器/最小系统）时
  available=false，巡检与前端跳过不报错（smart available=false 同款）
- 命令超时：单命令 5s，整次 RPC 30s 上限；argv 直调不经 shell，**只读命令白名单**，
  无参数注入面

## D3b M2：DSM provider（群晖）

DSM 6/7 通用 webapi 入口（`{addr}/webapi/entry.cgi`，全部 GET + query 参数，
`url.Values` 编码——密码含 `&=` 等特殊字符安全），单请求 5s 超时、尊重整体
30s ctx：

| 步骤 | API | 要点 |
|---|---|---|
| 登录 | `SYNO.API.Auth` v6 `login` | `account/passwd/session=StorageManager/format=sid` → `data.sid`；建议为 cockpit 建专用账号且不开 2FA（OTP 账号登录直接失败） |
| 池+卷 | `SYNO.Storage.CGI.Storage` v2 `load_info` | `pools[]`（id/status/devices/total_size/used_size）+ `volumes[]`（volume/status/fs_type/total_size/used_size/pool_id） |
| 共享 | `SYNO.FileStation.List` v2 `list_share` | `shares[]`（name/path）；DSM 共享文件夹协议粒度拿不到（SMB/NFS 权限在 per-share 内部 API），统一标 `smb` |
| 登出 | `SYNO.API.Auth` v6 `logout` | best-effort defer，失败不影响快照 |

后续请求统一带 `_sid`。字段映射：

- pools：status `Normal→healthy`、`Degrade→degraded`、`Crash(ing)→failed`、
  `Resync(ing)/Migrat(ing)→resync`、其余→unknown；`Kind="dsm"`；
  devices 原样、字节→GB
- volumes → Mounts（DSM 卷本质是挂载点，容量告警语义直接复用）：
  Device=pool_id、MountPath=/volume1、FsType=fs_type、字节→GB
- shares → Shares：Protocol=`smb`、Hosts 留空（NFS 规则在内部 API，不取）

错误纪律：

- 登录失败/HTTP 非 200/`success:false` → 该 target 降级（log 一条，含
  target 名与 DSM error code，**不含账号密码**）；DSM code 407 = 连续失败
  IP ban，log 特别标注提醒（不自动退避，巡检间隔本身 ≥5min 足够稀疏）
- target addr 必须带协议（`https://` 或 `http://`），缺协议条目丢弃
- 快照映射每条记录 `Host=target.Name`；凭据永不进快照

capability 语义不变：`DetectNas()` 仍只看本地工具（targets 是运行期配置，
配了 target 的 agent 天然有观测价值，但 capability 影响的是「这台 agent 有
NAS 可看」，保持探测可复现——无本地工具也无 target 的 agent 不注册）。

## D4 capability 与注册

- `DetectNas()`：mdadm/zpool/vgs/btrfs/testparm/exportfs 任一 LookPath 成功
  （纯容器最小系统不注册，避免全量 agent 冗余 capability）
- capability：`{Type: "nas", Version: "1"}`，detectCapabilities 追加（cron 同款）
- providers.go：`case "nas"` 注册 `NewNasProvider(NasConfig{})`
- RPC 单方法：`nas.status` → NasSnapshot（overlay.status 同款全量快照）

## D5 server：REST 与巡检告警

REST（照 smart 模式）：

```
GET  /api/agents/{id}/nas/status   纯转发 nas.status（浏览，不审计，不落库）
GET  /api/nas/config               巡检设置 + 阈值
PUT  /api/nas/config               保存（巡检配置类不记审计，smart/drift 同构）
```

巡检 `nasScanLoop`（smart/ddns/acme 同款骨架）：

- Setting `nas.scan_interval_seconds`：min 300 / max 86400 / 默认 1800，0=关闭
- 每轮对全部带 nas capability 的在线 agent 逐个拉快照，只判告警不落库
- 告警规则（`CheckNasScan`，真去重——resource+title 未读期间只报一次）：
  - 池 State ∈ {degraded, resync} → warning；failed → error（title 按池名区分）
  - 挂载点使用率 ≥ `nas.usage_warn_percent`（默认 80，50-99）→ warning，
    title 含挂载路径；单一挂载点恢复后自愈清错（复用告警去重语义）
  - available=false 与 unknown 态不告警（观测缺失≠故障，smart D9 同则）
  - 记录 `Host` 非空（网络 NAS）时告警 title 的「主机」位显示设备名
    （`存储池故障：volume_1（主机 home-dsm）`），去重天然隔离不同设备

## D6 Web /nas 页

- 菜单「存储池」（HddOutlined 与磁盘健康区分：磁盘健康看盘，存储池看卷），
  位于「磁盘健康」之后
- 跨 agent 总览（Disk 页同款布局）：
  - 异常置顶：degraded/resync/failed 池 + 超阈值挂载点聚合为顶部告警条
  - 按主机 Collapse 分组，每主机三段：
    - 池表：名称/Kind Tag/状态徽标（healthy 绿 degraded 橙 failed 红 resync 蓝）/容量条/设备/明细
    - 挂载表：设备/挂载点/FsType/容量条（≥阈值红显）
    - 共享表：协议 Tag/名称/路径/导出范围
  - agent 按 nas capability 过滤（cron 页同款下拉）
- 无 nas capability agent 时显示引导说明（探测条件 = 任一存储工具存在）

## D7 安全边界

1. agent 侧全部只读：读 /proc 文件 + 白名单只读命令，零写路径
2. argv 直调不经 shell；命令名固定，无参数来自用户输入
3. 快照不含任何凭据/密钥（SMB 只取共享名与路径，不取账号密码段）
4. server 纯转发不落库：NAS 本机是唯一事实源；巡检只产出告警
5. M2 网络 API 型凭据只存 agent 环境变量，与 OpenWrt/PVE 同纪律

## D8 测试策略

- **agent**（rpc/nas_provider_test.go）：fake Commander + 临时文件注入——
  /proc/mdstat 样例（active/resync/(F) 盘三态）、zpool list/status 样例、
  df -kP 样例（伪 FS 过滤断言）、testparm 共享段解析、exportfs 解析；
  单源失败不影响其余（注入一源报错）；全源缺失 → available=false；
  无注入面校验（命令名单固定断言）
- **agent DSM**（rpc/nas_dsm_test.go）：`httptest` 起**假 DSM**
  （handler 按 api/method 应答 entry.cgi JSON 样例）+ `t.Setenv`
  注入 `COCKPIT_NAS_TARGETS`——登录→load_info→list_share→登出全流程、
  池状态映射（Normal/Degrade/Crash/Resync）、字节→GB、volumes→Mounts、
  shares→smb、快照记录 Host=target 名、**快照与错误消息不含密码**；
  登录失败该 target 降级其余不受影响；targets JSON 非法/缺字段/条目
  丢弃；type 未实现的条目忽略
- **server**（api_nas_test.go + nas_scan_test.go）：转发 + config 校验
  （interval 0/300/86400 合法、299/86401 拒绝；阈值 50-99）；巡检告警——
  degraded 池告警 warning、failed 池 error、超阈值挂载点告警、恢复不再重复、
  available=false 不告警；nas_config_update 审计恰一条
- **web**：`npx tsc --noEmit`（新增文件零错误）+ `npm run build`

## D9 验收清单

- [ ] `go test ./internal/agent/rpc/ ./internal/server/` 通过
- [ ] 新建文件 gofmt 干净；`go build ./...` 通过
- [ ] web tsc + build 通过
- [ ] 真实环境验收（列入 todo.md）：有 mdadm/ZFS/大容量挂载的主机实测池状态
      与容量、SMB/NFS 共享解析、告警真去重；M2 各 NAS 真机 API 验收
