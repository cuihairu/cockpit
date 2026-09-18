# 服务管理设计（systemd + Windows SCM）

> 状态：M1 设计定稿（systemd）；M2 追加 Windows SCM 后端（D9），capability
> 统一为 `service`。Agent 侧按平台后端各一个 provider；server 纯转发不落库
> （systemd / SCM 为各自事实源）；Web /services 页照 cron 页模式。

## D1 目标与范围

**M1 交付**：systemd 主机上 `*.service` unit 的列表观测 + 运行时操作：

- 列出服务：加载/激活状态、是否自启（unit file 状态）、描述
- 概览：systemd 整体运行状态（running / degraded / ...）+ active/failed 计数
- 操作：start / stop / restart / reload（运行时），enable / disable（自启持久化）

**M1 不做**（后续候选）：

- journal 日志查看（logs provider 已覆盖 journalctl 查询）
- timer / socket / mount 等 unit 类型（只做 `.service`）
- mask / unmask、unit 文件编辑、daemon-reload（M2 候选）
- 非 systemd 平台：macOS launchd（M3 候选，见 D9 范围说明）；OpenRC / runit 不做

定位：NAS 场景前置能力——服务启停与自启管理是主机运维的基本盘，模式与 cron 页同构，交付确定性高。

## D2 探测与 capability

```go
// DetectSystemd 探测 systemd 可用性
func DetectSystemd() bool {
    if _, err := exec.LookPath("systemctl"); err != nil {
        return false
    }
    fi, err := os.Stat("/run/systemd/system")
    return err == nil && fi.IsDir()
}
```

- `/run/systemd/system` 存在是经典的"本机 systemd 已启动"判断：排除容器内装了
  systemctl 二进制但无 systemd PID 1 的场景，以及 chroot / 救援环境
- capability：`{Type: "systemd", Version: "1"}`，追加在 agent.go detectCapabilities
  （与 cron capability 同款做法，无需专门 detector 包条目）
- providers.go：`case "systemd"` 注册 `NewServiceProvider(nil)`
- 权限：操作 system 级 unit 需要 root（agent 常规以 root 运行，与备份/nginx reload
  同前提）。非 root 时 systemctl 报错，agent 原样透传 stderr 摘要，不做 sudo 提权

## D3 RPC 方法（provider type = `service`）

```go
type Commander func(ctx context.Context, name string, args ...string) (stdout []byte, stderr []byte, err error)
```

复用 nginx_provider.go 的 Commander 抽象与 defaultCommander，argv 直传不经 shell。

### service.list — 服务列表

两轮查询合并（任一轮失败即报错）：

1. `systemctl list-units --type=service --all --no-legend --no-pager`
   列：UNIT LOAD ACTIVE SUB DESCRIPTION（只含已加载的 unit，含 inactive）
2. `systemctl list-unit-files --type=service --no-legend --no-pager`
   列：UNIT FILE STATE VENDOR PRESET（含未加载的全部已安装 unit）

按 unit 名合并：list-units 提供运行态（load/active/sub + description），
list-unit-files 补充未加载的安装项并提供自启态（enabled/disabled/static/...）。
未加载项 activeState 记为 `inactive`。列表按名称排序返回：

```json
{ "services": [ { "name": "nginx.service", "description": "...",
    "loadState": "loaded", "activeState": "active", "subState": "running",
    "unitFileState": "enabled", "preset": "enabled" } ] }
```

解析：列间按空白切分，DESCRIPTION 为剩余列合并（`strings.SplitN`）。
`--no-pager` 防 systemctl 在非 tty 下分页挂死；`--no-legend` 去表头脚注。
典型主机几十~几百个 service，全量返回（数十 KB 量级）不做分页。

### service.status — 概览

- `systemctl is-system-running`：输出 running / degraded / maintenance /
  stopping / offline 等。注意 degraded 时退出码非 0 但 stdout 有值——以 stdout
  为准，err 仅在 stdout 为空时才视为失败（探测不可用）
- 服务计数：从 service.list 结果统计 active / failed / enabled 数
- 返回 `{ "systemState": "degraded", "total": 87, "active": 62, "failed": 1, "enabled": 40 }`

### service.action — 执行操作

params：`{ "name": "nginx.service", "action": "restart" }`

- **动作白名单**：start / stop / restart / reload / enable / disable（M2 再议 mask/unmask）
- **unit 名白名单**：正则 `^[A-Za-z0-9@._+-]+$` 且必须以 `.service` 结尾；
  排除 `/`、空格、`..` 等一切歧义字符。前端传参自动补 `.service` 后缀
- 执行 `systemctl <action> <name>`，超时 30s（restart 数据库类服务可能慢）
- 成功返回 `{ "name": "nginx.service", "action": "restart" }`；失败原样透传
  stderr 摘要（commandErrSummary），如 `Failed to restart nginx.service: ...`

## D4 安全边界

1. 只暴露 systemctl 六个动词，无任意命令执行面
2. unit 名正则 + `.service` 后缀强制，双端同规则校验（server 拒一次、agent 拒一次）
3. Commander 模式 argv 直传，参数不经过 shell，无注入可能
4. 审计：action 类操作记 `service_action`（details 带 action 动词与 agent ID）；
   list / status 浏览不记审计（cron 先例）。enable/disable 虽是持久化变更，
   同记 service_action，details.action 区分
5. systemd 本身是事实源与权限边界：agent 只能操作其运行身份有权操作的 unit，
   cockpit 不做提权

## D5 server REST 端点

挂在 serveAPI 的 `/agents/` 分支下（api.go：`strings.Contains(agentID, "/services/")`），
handler 文件 api_service.go，模式照 api_cron.go（纯转发不落库，D9 同款纪律）：

```
GET  /api/agents/{id}/services/status            → service.status（浏览，不审计）
GET  /api/agents/{id}/services                   → service.list（浏览，不审计）
POST /api/agents/{id}/services/{unit}/{action}   → service.action（审计 service_action）
```

- `{unit}` 与 `{action}` 双端同规则校验（白名单正则 + 动作白名单），
  非法请求 server 直接 400，不消耗 agent 往返
- `{action}` 放 URL 路径（动词即资源语义），unit 名含 `.` 无碍 path 解析
- agent 离线 → 503（registry.Get 检查）；agent 侧错误原样透传 502
- audit 常量：`ResourceService = "service"`，动作 `service_action`，
  resourceID = unit 名

## D6 Web /services 页

- 路由 `/services`，菜单「服务管理」（ThunderboltOutlined），位于「定时任务」之后
- agent 选择：照 cron 页——下拉列出注册 agent，标注 systemd capability；
  未检测到 systemd capability 的 agent 不可选（或选中后显示引导卡片）
- 状态卡：systemState 徽标（degraded 橙 / running 绿）+ total/active/failed 计数
- 服务表：
  - 名称（code 样式）+ 描述
  - Active 徽标：active 绿 / activating 蓝 / failed 红 / inactive 灰 / 其他默认
  - 自启 Tag：enabled 绿 / disabled 灰 / static 蓝（tooltip：静态依赖，不可直接启停管理）
  - 操作按钮（按状态显隐）：active → 停止/重启；inactive → 启动；
    unitFileState enabled ↔ disabled → disable/enable 切换；reload 仅 active 显示
  - 动作进行中行级 loading；完成后 invalidate 列表 + 状态卡
- 列表 30s 自动轮询（refetchInterval），操作后立即刷新
- 无 systemd agent 时显示引导卡片（说明探测条件与 agent 权限要求）

## D7 测试策略

- **agent**（rpc/service_provider_test.go）：fake Commander 注入（cron 测试同款）——
  systemctl 双命令输出样例文本 → 合并解析断言（含未加载项、DESCRIPTION 含空格、
  排序）；action 白名单拒绝；unit 名非法拒绝（`../`、空格、无 .service 后缀）；
  action 失败透传 stderr；is-system-running 非零退出码但有输出 → 不报错
- **server**（api_service_test.go）：双端校验（坏 unit 名 / 非法 action → 400）；
  离线 agent → 503；转发成功 + service_action 审计恰好一条（details 含 action）；
  list/status 不产生审计
- **web**：`npx tsc --noEmit` + `npm run build`

## D8 验收清单

- [ ] `go test ./internal/agent/rpc/ ./internal/server/` 通过
- [ ] `go vet ./...` 干净；新建文件 gofmt 干净
- [ ] web tsc + build 通过
- [ ] 真实环境（systemd 主机）：列表含未加载 unit、restart/enable 实测、
      非 root agent 报错透传（列入 todo.md）

## D9 跨平台统一：Windows SCM 后端（M2）

### D9.1 动机与统一模型

Windows 的服务控制管理器（SCM）与 systemd 语义高度同构：服务有运行态与
自启态，支持启停与自启切换。两者存在最小公共集，观测模型直接复用
`ServiceUnit`，不新增 DTO：

| ServiceUnit 字段 | systemd | Windows SCM |
|---|---|---|
| Name | `nginx.service` | SCM 服务名（如 `wuauserv`，无后缀） |
| Description | unit 描述 | DisplayName |
| LoadState | loaded/not-found | 恒 `loaded`（服务必然注册在 SCM） |
| ActiveState | active/inactive/failed/... | 状态映射（见 D9.3） |
| SubState | systemd 细分态 | SCM 原始状态小写（`running`/`stopped`/`paused`...） |
| UnitFileState（自启态） | enabled/disabled/static/... | StartType 映射（见 D9.3） |
| Preset | vendor preset | 空（Windows 无预设概念，前端 `—` 兜底） |

生态选型：Go 官方 `golang.org/x/sys/windows/svc/mgr`（纯 syscall，
CGO_ENABLED=0 可用，与 Windows 构建矩阵兼容）。kardianos/service 语义是
"把自己装成服务"，与"管理既有服务"不同，不采用；gopsutil 只有进程视图，
无自启态。macOS launchd 语义差异大（LaunchAgent/LaunchDaemon 双域、
plist 文件即事实源），留 M3 单独设计，不在本节范围。

### D9.2 capability 统一与探测

capability 由 `systemd` 更名为 `service`，后端差异放 metadata：

```json
{ "type": "service", "version": "2", "metadata": { "backend": "systemd" } }
{ "type": "service", "version": "2", "metadata": { "backend": "windows-scm" } }
```

- 更名理由：agent/server/前端同仓同发无兼容包袱；"systemd" 作为 capability
  名会挡住 Windows 主机，而 `service` 才是能力本身的语义
- Linux/macOS：沿用 DetectSystemd（systemctl + /run/systemd/system），
  探测通过才上报 capability，backend=systemd
- Windows：SCM 是操作系统内置组件恒存在，无需文件探测，直接上报
  backend=windows-scm
- providers.go `case "service"`：按 metadata.backend 构造对应 provider
  （systemd → `NewServiceProvider(nil)`；windows-scm → `NewWindowsServiceProvider()`）
- 日志页（logs provider）的 `systemd` 日志源类型与本 capability 无关，不改

### D9.3 字段与动作映射

**运行态映射**（`windowsStatusToActiveState`，纯函数）：

| SCM Status | ActiveState | SubState |
|---|---|---|
| Running | active | running |
| Stopped | inactive | stopped |
| Start Pending | activating | start_pending |
| Stop Pending | deactivating | stop_pending |
| Paused / Pause Pending | paused | paused / pause_pending |
| Continue Pending | activating | continue_pending |

`paused` 不映射为 active：暂停是 Windows 特有语义，前端未知值按灰色徽标
显示原词，不扭曲语义。

**自启态映射**（`startTypeToUnitFileState`，纯函数）：

| SCM StartType | UnitFileState |
|---|---|
| Automatic（含 DelayedAutostart） | enabled |
| Manual | disabled |
| Disabled | disabled |

前端「自启」列只用 enabled/disabled 两态，Manual 与 Disabled 同为「手动」。

**动作映射**（Windows 支持 6 动词中的 5 个，`reload` 不支持）：

| 动作 | SCM 实现 | 说明 |
|---|---|---|
| start | `svc.Start()` | 即发即返，状态收敛靠前端 30s 轮询 |
| stop | `svc.Stop()` | 同上；SCM 的启停是异步请求，与 systemd 同步语义有差异 |
| restart | Stop → 等待 Stopped（30s 超时）→ Start | SCM 无原生 restart |
| reload | — | 返回错误 "reload is not supported on windows-scm backend"；前端按 backend 隐藏按钮 |
| enable | `SetStartType(Automatic)` | 等价 systemd enable 的开机自启 |
| disable | `SetStartType(Disabled)` | 等价 systemd disable |

超时复用 `serviceActionTimeout`（30s）。

### D9.4 安全边界

1. **Windows 服务名白名单**（与 systemd 正则分开，`validateWindowsServiceUnit`）：
   正则 `^[A-Za-z0-9_.\- ]{1,256}$` 且不得为 `.` / `..`（SCM 注册表键名字符集，
   不含 `/` `\` 与通配符）。服务名与 DisplayName 是两回事，操作一律按服务名
2. 动作白名单：systemd 6 动词；Windows 5 动词（无 reload），provider 内各自校验
3. **server 端校验放宽为双规则并集**：`{unit}` 满足 systemd 正则 **或** Windows
   正则其一即放行转发。server 不解析 agent capability 做精确匹配——并集仍是
   纯白名单（无 `/` `\` `..` 空字节），真正的后端专属严格校验仍在 agent 侧
   兜底（systemd 侧拒绝无 `.service` 后缀，Windows 侧拒绝 `.service` 后缀外的
   越界字符）；动作白名单不变
4. SCM 权限边界：agent 运行身份对服务的写权限由 Windows ACL 决定，cockpit
   不提权；无权限时 SCM 报错原样透传
5. 审计不变：action 记 `service_action`，list/status 不审计

### D9.5 实现组织（跨平台编译）

三平台 CI 矩阵下所有 agent 代码全平台编译，Windows-only 代码用 build tag 隔离：

```
service_provider.go        无 tag：systemd 实现（现有，不改逻辑）
service_windows_model.go   无 tag：WinService DTO → ServiceUnit 映射纯函数 +
                                   validateWindowsServiceUnit（Linux CI 可测）
service_windows_scm.go     //go:build windows：mgr.Connect 采集 + 动作真实现
service_windows_stub.go    //go:build !windows：NewWindowsServiceProvider 返回
                                   stub，Call 显式报错（防御层：非 Windows
                                   平台探测门控根本不会注册它）
```

`x/sys` 已在依赖树（indirect），实现后转直接依赖。采集规模：SCM 本地 RPC
枚举数百服务为毫秒级，与 systemctl 全量列表同量级，一次性全量返回不分页。

### D9.6 Web 适配

- agent 下拉过滤改 `c.type === 'service'`，从 capability metadata 读 backend
- backend=windows-scm 的主机：隐藏「重载」按钮、「系统状态」概览项
  （status 返回 systemState=`unknown`，SCM 无 systemd 全局状态概念）
- 名称列 `replace(/\.service$/, '')` 对 Windows 名无副作用；「自启」列
  Windows 只会出现 enabled/disabled 两态，渲染逻辑复用
- 下拉未检测文案改「未检测到服务管理」

### D9.7 测试策略

- **agent**（service_windows_model_test.go，Linux CI 可跑）：运行态/自启态映射
  表驱动全分支；validateWindowsServiceUnit 合法名/含 `/`/`..`/超长/reload 拒绝
- **stub**：非 Windows 平台 Call 显式错误（CI 平台即测）
- **server**（api_service_test.go 补例）：Windows 名（`wuauserv`，含连字符与
  空格样例）转发 200；仍非法的名（`../x`、空字节）400
- **systemd 回归**：现有 service_provider_test.go 不动即过（provider 逻辑未变）
- **真机验收**（列入 todo.md）：Windows 主机列表/启停/自启切换/restart 等待、
  无权限服务报错透传；reload 按钮确认隐藏

### D9.8 验收清单

- [ ] `GOOS=windows go build ./...` 通过；`go test ./...`（Linux）通过
- [ ] `go vet ./...` 干净；新建文件 gofmt 干净
- [ ] web tsc + build 通过
- [ ] Windows 真机验收项列入 todo.md

## D10 macOS launchd 后端（M3）

### D10.1 范围与探测

- 只管 **system 域 LaunchDaemons**（`/Library/LaunchDaemons` +
  `/System/Library/LaunchDaemons`；agent 以 root 运行的前提与 systemd 侧同）。
  LaunchAgents（用户会话域，gui/<UID>）不做——域前缀依赖用户 UID，语义另议
- 探测：`runtime.GOOS == "darwin"` 恒注册，capability 同 D9.2 统一模型
  `{type: "service", metadata: {backend: "launchd"}}`（launchd 是 PID 1 无需文件探测）
- D1 的「macOS launchd M3 候选」至此关闭；OpenRC / runit 仍不做

### D10.2 观测模型映射（复用 ServiceUnit）

| ServiceUnit 字段 | launchd 来源 |
|---|---|
| Name | plist `Label` 键（如 `com.example.ssh`，≠文件名） |
| Description | plist 文件路径（launchd 无描述概念，路径即「这个 daemon 是谁」的线索） |
| ActiveState | `launchctl list` PID>0 → active；Status 非 0（上次异常退出）→ failed；其余（含仅安装未加载）→ inactive |
| SubState | 空（launchd 无细分态） |
| UnitFileState（自启态） | `RunAtLoad && !Disabled` → enabled，否则 disabled |
| Preset | 空 |

两源合并（systemd list-units/list-unit-files 同构）：

1. `launchctl list`（三列 PID/Status/Label，TSV）→ 运行态；Status 为 `-` 表示
   从未运行（inactive）
2. 扫描 LaunchDaemons 目录 `*.plist` → 全量安装项 + RunAtLoad/Disabled 键。
   plist 是 XML，标准库 `encoding/xml` 轻量解析顶层 dict 的
   Label/RunAtLoad/Disabled 三键即可，**不引入第三方 plist 库**；
   解析失败的文件跳过（log），全部失败不报错（目录可空）

### D10.3 动作映射（5 动词，`system/<label>` 服务目标）

| 动作 | launchctl 实现 | 说明 |
|---|---|---|
| start | `kickstart system/<label>`，失败回退 `bootstrap system <plist>` | kickstart 仅对已加载服务有效；bootout 过的需重新 bootstrap |
| stop | `bootout system/<label>` | launchd 无 stop 动词，卸载即停；plist 仍在（对比 systemd stop 不卸载，语义差异见下） |
| restart | `kickstart -k system/<label>` | 原生 kill+重启 |
| reload | — | launchd 无 reload 语义，报错同 Windows SCM |
| enable | `enable system/<label>` | 清 disabled DB 标记 |
| disable | `disable system/<label>` | 写 disabled DB 标记；**不卸载正在运行的实例**（与 systemd disable 立即生效语义不同，前端提示文案已通用化不涉及） |

超时复用 serviceActionTimeout；Commander 抽象复用（argv 直调 launchctl 不经 shell）。

### D10.4 安全边界

1. **label 白名单**（`validateLaunchdLabel`）：`^[A-Za-z0-9_][A-Za-z0-9._-]{0,254}$`
   （反向域名惯例字符集，不含 `/` 与空格）+ 动作白名单 5 动词；
   与 systemd/Windows 校验三足鼎立，agent 侧按 backend 各自严格校验
2. server 端并集白名单已天然覆盖 label 字符集（匹配 Windows 名正则），零改动
3. system 域动作需要 root；权限错误原样透传
4. 审计/浏览纪律不变（action 记 service_action）

### D10.5 实现组织与 Web

- 三层组织同 D9.5：`service_launchd_model.go`（无 tag：plist 解析 +
  launchctl list 解析 + 映射 + 校验纯函数，Linux CI 可测）、
  `service_launchd_darwin.go`（//go:build darwin 真执行）、
  `service_launchd_stub.go`（//go:build !darwin 防御 stub）
- Web：D9.6 的 `isWindows` 特判泛化为「backend === 'systemd' 才显示重载
  按钮与系统状态项」——launchd 与 windows-scm 同隐藏；status 的
  systemState 恒 `unknown`（launchd 无全局 degraded 概念）
- capability 过滤、下拉文案、名称列渲染零改动（`replace(/\.service$/)`
  对 label 无副作用）

### D10.6 测试策略

- **模型层**（service_launchd_model_test.go，Linux CI）：plist XML 解析
  （RunAtLoad/Disabled 有无组合、缺 Label 跳过、格式坏文件）、launchctl list
  输出解析（PID/-/非零 Status）、自启态映射、label 校验（合法/`/`/空格/超长）、
  动作白名单（5 动词过、reload 拒）
- **stub**：非 darwin 平台 Call 显式报错
- **server/web**：零改动即回归；`GOOS=darwin go build` 编译验证
- **真机验收**（列入 todo.md）：macOS 主机列表/启停/自启切换、
  kickstart 回退 bootstrap 路径、无权限报错透传

