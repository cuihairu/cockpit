# systemd 服务管理设计

> 状态：M1 设计定稿。Agent 侧 provider 复用 Commander 抽象直调 systemctl；
> server 纯转发不落库（systemd 为唯一事实源）；Web /services 页照 cron 页模式。

## D1 目标与范围

**M1 交付**：systemd 主机上 `*.service` unit 的列表观测 + 运行时操作：

- 列出服务：加载/激活状态、是否自启（unit file 状态）、描述
- 概览：systemd 整体运行状态（running / degraded / ...）+ active/failed 计数
- 操作：start / stop / restart / reload（运行时），enable / disable（自启持久化）

**M1 不做**（后续候选）：

- journal 日志查看（logs provider 已覆盖 journalctl 查询）
- timer / socket / mount 等 unit 类型（只做 `.service`）
- mask / unmask、unit 文件编辑、daemon-reload（M2 候选）
- 非 systemd 平台（OpenRC / runit / macOS launchd）

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
