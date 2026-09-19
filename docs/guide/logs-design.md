# P2 方案设计：日志查看（远程 journalctl / docker logs 查询器）

> 2026-09-15。P2「日志聚合查看」首期设计。todo.md 原文：「Agent 推送容器/系统
> 日志，WebUI 统一检索（可接 Loki/轻量自研）」。本文只做设计决策与接口定义。

## 现状与痛点

| 痛点 | 现状 | 后果 |
|------|------|------|
| 「服务为什么挂了」要 SSH 逐台查 | journalctl / docker logs 是命令输出，不是文件，文件管理器覆盖不了 | 排障路径长，多机场景逐台登录 |
| systemd 服务日志无入口 | Workbench 只有终端/桌面/文件 | 不知道服务何时开始异常、报了什么 |
| 容器 stdout 日志无入口 | Docker 页面只有容器列表与生命周期操作 | 容器崩溃只能进终端 docker logs |
| cron 执行失败无处可查 | cron-design.md D10 明确执行历史属日志范畴 | 定时任务静默失败（journal 里有 cron.log） |

## 架构总览

与反向代理/定时任务同一通道模型：**同步 RPC + server 纯转发 + 不落库**。
M1 是查询式查看器（pull 型）：每次查询由 agent 实时执行 journalctl / docker logs，
无采集、无存储、无推送——聚合采集管道（Loki 式）留 M2。

```
┌─ server ────────────────────┐        ┌─ agent（日志所在主机）────────────┐
│ REST /api/agents/{id}/logs  │ ─RPC─▶ │ logs provider（新）                │
│ 纯转发，不落库              │ ◀─RPC─ │ journalctl / docker logs 实时查询  │
│ 浏览类操作不记审计          │        │ 白名单校验 + 字节上限 + 超时       │
└─────────────────────────────┘        └───────────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | M1 边界 | 查询式查看器（pull）：agent 不采集不存储，查询时实时执行命令 | 个人场景日志查询频率低，实时执行毫秒级返回即可闭环；采集管道 + 跨机检索是独立量级，M2 立项 |
| D2 | 日志源范围 | systemd 服务（journalctl）+ docker 容器（docker logs）两类；文件日志走既有文件管理器不重复 | 覆盖系统级与容器级两大入口；journal 顺带覆盖 cron/ssh 等系统日志 |
| D3 | capability | `logs`：journalctl 或 docker 至少一个可用（LookPath）才注册；status 返回两者可用性 | 与 cron D7 同款纪律，不出现假能力；单源主机照样可用 |
| D4 | 查询参数 | `{type, source, tail, since_minutes, grep}`：tail 默认 200 上限 2000 行；since_minutes 最近 N 分钟（0=不限）；grep 关键词 agent 侧内存过滤 | tail+since+grep 覆盖九成排障查询；grep 用统一内存 contains 过滤而非 journalctl -g（PCRE 语义）/docker logs 无内建——**双端行为一致且可测** |
| D5 | 安全边界 | source 名白名单正则 `^[a-zA-Z0-9][a-zA-Z0-9_.@+-]{0,127}$`；输出上限 1MB（超限截断 + truncated 标记）；命令超时 30s；argv 直传不经 shell | 纵深防御：即便正则放行也不会注入（不经 shell）；1MB/2000 行防大日志拖垮 WS；读日志与终端权限同级，沿用既有 agent 权限模型，不另设 RBAC |
| D6 | 输出形态 | 整段文本（string，short-iso/`--timestamps` 统一带时间戳）+ truncated 标志 | 行数组 JSON 体积大、前端反而要 join；整段文本 pre 渲染 + 前端 grep 高亮即可 |
| D7 | RPC 方法 | `logs.status` / `logs.sources` / `logs.query` 三方法 | 最小闭环：可用性、源枚举、查询 |
| D8 | REST 挂载 | `/api/agents/{id}/logs/*`，复用 serveAPI `/agents/` 分发模式 | 按 agent 作用域；纯转发不落库（与 cron D9 同理） |
| D9 | 审计 | 查询类操作不记审计（浏览性质，与文件浏览同纪律） | M1 无变更类操作；若未来加「日志下载/导出」再补 |
| D10 | 源枚举实现 | systemd：`systemctl list-units --type=service --no-legend --plain` 取 unit 名；docker：<span v-pre>`docker ps --format '{{.Names}}'`</span>（行内代码无 v-pre，Go template 花括号须手动包裹防 Vue 插值解析） | 只列**运行中**对象（查询的是活日志）；`--all` 扩展按需后补 |

## Agent 侧设计

### 探测与注册

- `DetectLogs() (journalctl, docker bool)`：`exec.LookPath` 双探测；
  两者皆缺 → 不追加 `logs` capability、不注册 provider；
- provider 复用 nginx/cron 的 `Commander` 抽象，测试可注入。

### RPC 方法

| 方法 | 参数 | 返回 | 说明 |
|------|------|------|------|
| `logs.status` | `{}` | `{journalctl, docker}` | 两类源可用性 |
| `logs.sources` | `{}` | `{systemd: [unit...], docker: [name...]}` | 运行中对象枚举（任一类不可用返回空数组） |
| `logs.query` | `{query}` | `{lines, truncated}` | 见下 |

### query 参数与校验（server 双端同规则）

| 字段 | 规则 |
|------|------|
| `type` | `systemd` / `docker` |
| `source` | 白名单正则（D5），systemd 侧允许 `.service` 等后缀 |
| `tail` | 1-2000，默认 200 |
| `since_minutes` | 0-1440（0 = 不限，默认 0） |
| `grep` | 可选，≤256 字符，不含换行（逐行 contains） |

### 命令构造

```
systemd：journalctl -u {source} -n {tail} --no-pager -q -o short-iso [--since=-{N}min]
docker ：docker logs --tail {tail} --timestamps [{--since Nm}] {name}
```

- 输出处理：读 stdout → 按 grep 过滤 → 超 1MB 截断到最后一行边界 → `truncated=true`；
- grep 过滤后为空属正常结果（`lines: ""`），非错误。

## Server 侧设计

### REST API（server/api_logs.go，挂 serveAPI `/agents/` 分支，JWT）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/agents/{id}/logs/status`  | 转发 logs.status |
| GET  | `/api/agents/{id}/logs/sources` | 转发 logs.sources |
| POST | `/api/agents/{id}/logs/query`   | body 为查询参数 → 双端校验 → 转发 logs.query |

- 参数校验双端同规则（D5）；agent 错误原样透传（502）；离线 503。

## Web UI 设计

- Workbench 新增「日志」Tab（无连接语义，与「文件」同款内嵌面板）：
  - **源选择**：类型 Segmented（systemd 服务 / docker 容器）→ 具体源 Select
    （来自 sources，随类型联动）；无可用源时给出 capability 提示；
  - **过滤栏**：最近时长 Select（不限/5 分钟/1 小时/24 小时）、行数 InputNumber
    （≤2000）、关键词输入（回车即查）、查询按钮 + 刷新按钮；
  - **日志视图**：等宽 pre、深色底、自动滚动到底部；grep 命中高亮；
    ERROR/FATAL 行红、WARN 行橙（前端正则着色，不改 agent 输出）；
  - **状态行**：返回行数 / 截断提示（`已截断，仅显示最后 N KB`）/ 查询耗时。

## M2：实时尾随（2026-09-18）

tail -f 式流式查看：agent 侧跟随进程持续推 chunk，server 经 HTTP 流式响应转发浏览器，断开即停。数据面复用 ProxyData 通道（与 terminal 同构），控制面走常规 RPC——**协议零新增**。

| D | 决策 | 理由 |
|---|------|------|
| F1 | 通道拆分 | **控制面** RPC：`logs.follow.start`（`{followId, type, source, tail, grep}`，返回成功即开始）/ `logs.follow.stop`（`{followId}`）；**数据面** 复用 `proxy_data`/`proxy_close` 消息，`proxyId = "logs:<followId>"`——与 terminal 前缀模式同构，协议不加新类型 | RPC 校验/错误/测试走既有惯例；数据单向高频，ProxyData 已是二进制高效路径 |
| F2 | agent 跟随进程 | systemd：`journalctl -f -n $TAIL -o short-iso -u $UNIT`；docker：`docker logs -f --tail $TAIL --timestamps $NAME`；bufio 按行扫描（行上限 1MB 同 query），grep 内存 contains 同款过滤 | 命令拼装复用 query 同款参数语义，行为一致；argv 直传不经 shell |
| F3 | agent 会话上限 | 单会话累计输出 4MB 即停（发 close reason=limit）；会话超时 10 分钟自动停；follows map 上限 16 个；followId 重复 start 覆盖旧会话（先杀旧进程） | 防打爆 WS 与浏览器内存；上限内个人场景富余 |
| F4 | server 流式端点 | `POST /api/agents/{id}/logs/follow`（body 同 query 参数），响应 NDJSON 流：`{"data":"..."}` 数据帧 / `{"eof":true,"reason":"..."}` 终止帧；认证走既有 JWT 中间件（fetch POST 可带头，不用 EventSource） | fetch streaming（ReadableStream）现代浏览器全支持；abort() 即断开，客户端关闭 → server 检测 ctx.Done → 发 follow.stop |
| F5 | server 生命周期 | `logsFollowers` 注册表（followId → chan + agentID）；ProxyData `logs:` 前缀分派写入；ProxyClose 同前缀 → 流发 eof 帧收尾；全局 follow 上限 8 / 每 agent 上限 2；CallAgent 失败即 4xx/5xx 不占表 | 与 terminalSessions 同款注册表模式；上限防滥用 |
| F6 | 语义细节 | start 后 agent 先推回填 tail 行再持续跟随（journalctl -f 自带、docker logs -f --tail 自带）；docker 容器停止 → 进程退出 → ProxyClose reason=exited；server 校验与 query 完全同规则（source 白名单/tail 上限/grep 无换行） | tail -f 心智：回填 + 跟随一体 |
| F7 | web | LogsPanel「实时尾随」开关：开启清空当前视图 → fetch POST 流式读（AbortController），逐行追加渲染（复用 GrepLine 着色与 grep 高亮）；尾随中锁定源/参数输入；关闭/切源/组件卸载 abort | 单向流 + abort 足够，不引入 WS 复杂度 |
| F8 | 审计 | 尾随不记审计（同查询浏览纪律，D9） | 无变更操作 |

**测试**：agent follow 会话（mock Commander 输出逐行推、grep 过滤、4MB 上限停、stop 杀进程、followId 覆盖）；server 端点（校验 400、上限 429、NDJSON 帧序列、客户端断开触发 stop RPC、ProxyClose 收尾）；web build。

## M3：跨机联邦检索（设计，2026-09-19）

todo 原文「推送式采集与跨机检索、结构化解析」。设计期先回答路线问题——「跨机检索」
不必依赖「推送式采集先行」：

| 路线 | 形态 | 量级 | 结论 |
|------|------|------|------|
| (a) 联邦扇出检索 | server 并行向在线 agent 转发**既有** `logs.query`，按主机分组返回 | 小：1 个 server 端点 + 1 个 web 页，agent 零改动、零存储 | ✅ 选为 M3 本体 |
| (b) 推送式采集 + 服务端索引 | agent 常驻 tail → 推送 → server SQLite 落库 → 全文检索 | 独立量级：采集会话、断线补采、背压丢弃、保留清理、查询分页 | 维持不做，触发条件见「不做」清单 |

选 (a) 的理由：

- D1「查询频率低，实时执行即可闭环」在跨机维度同样成立——排障的高频动作是
  「同时在所有机器上 grep 同一条错误」，不是「对全量历史建索引」；
- 原判断「跨机统一检索：依赖采集管道先行」被联邦路线打破：零采集零存储同样
  闭环，且完整保持 D1/D8 纯转发哲学（server 依旧不落盘）；
- 离线机器的日志本来就无法访问（agent 是唯一通道），采集管道在个人云场景
  买不回「离线机器历史可查」这个能力；
- server 虽有 SQLite 先例（probe_results/alert），但 3-10 台机器的日志量撑不起
  索引收益，运维负担（容量/清理/迁移）先行到来。

结构化解析（journalctl -o json 字段提取）维持不做：docker 源无对应语义，做了要
双轨解析，文本形态 + grep + 前端着色（M1）已够用。

| D | 决策 | 理由 |
|---|------|------|
| D11 | 全局端点 `POST /api/logs/search`（不挂在 `/api/agents/{id}` 下）：对每个目标 agent 转发既有 `logs.query`，每机单源单查询；tail 收窄 ≤500/机 | 复用 M1 全部积木与安全边界（源白名单/1MB 截断/30s 超时/argv 直传）；扇出是放大器，10 机 × 500 行渲染与传输均可控 |
| D12 | 目标 = 在线且带 `logs` capability 的 agent；body.agents 指定子集时取交集；被排除目标回 `skipped` 并归因（offline / no-logs / not-found）；server 扇出并发上限 4，端点全局同时 ≤2 个在途请求，超出 429 | `Registry.List` + capability 过滤现成；双重闸门防扇出放大；个人场景 2 个并发检索富余 |
| D13 | 单 agent 失败/超时不整体失败：该机结果 `ok=false` + error 文本；全部目标不可用 → 200 空结果 + skipped 说明（不 5xx）；单目标超时 35s（agent 30s + 余量） | 与 NAS 单数据源降级（D8）、logs 多源降级（D2）同纪律；检索是浏览性质，部分成功仍是有效结果 |
| D14 | 结果按 agent 分组返回，**不做跨机时间归并**；每机 `{agentId, hostname, ok, truncated, output}`，output 与 `logs.query` 同形态 | 各机时钟不可信，跨机归并是伪精度；分组卡片内单机时间线自然有序 |
| D15 | 参数校验 server 侧镜像 M1 同规则：type 枚举、source 必填过白名单转发校验、grep ≤256 无换行、since_minutes 1-1440、tail 1-500（收窄于单机的 2000）；浏览性质不记审计（D9 同纪律） | 双端防御一致；扇出无写路径，审计只会刷屏 |
| D16 | web 独立「日志检索」页（新路由 + 菜单项）：表单（type / source 手填 AutoComplete / tail / since / grep / agent 多选默认全选）→ 按主机分组卡片 + grep 命中高亮；Workbench 单机「日志」Tab 保留（深查 + 实时尾随动线） | 跨机检索与单机深查是两条动线；source 跨机下拉需 N 次 sources 枚举（又一个扇出放大器），手填 AutoComplete 回避；着色与高亮复用 LogsPanel 既有实现 |

API 形态：

```
POST /api/logs/search
{"type": "systemd", "source": "nginx.service", "grep": "ERROR",
 "since_minutes": 60, "tail": 200, "agents": ["a1", "a2"]}   // agents 缺省 = 全部

200 →
{"results": [
   {"agentId": "a1", "hostname": "web-1", "ok": true,
    "truncated": false, "output": "...同 logs.query 形态..."},
   {"agentId": "a2", "hostname": "db-1", "ok": false, "error": "timeout"}],
 "skipped": [{"agentId": "a3", "reason": "offline"}]}
```

### Server 侧

- 新文件 `api_logs_search.go`：目标筛选（capability 过滤 / agents 交集 / skipped
  归因）→ 信号量并发 4 扇出 `CallAgent("logs.query", …)` → 单机结果透传 +
  hostname 标注；serveAPI 挂独立 `/api/logs/search` 分支（非 `/agents/` 前缀）；
- 端点级在途计数（atomic）≤2，超出 429；每目标 `context.WithTimeout(35s)`，
  单目标失败不传染。

### Web 侧

- 新路由 + 菜单「日志检索」页：`api.ts` 加 `searchLogs(payload)`；表单提交后
  按 `results` 顺序渲染主机卡片（ok=false 红色 error 态、skipped 灰色折叠行），
  卡片内复用 LogsPanel 的日志行着色与 grep 命中高亮；
- 表单校验与 D15 同规则前置（source 必填、grep 长度、tail/since 范围）。

**测试**：server `api_logs_search_test.go`——参数校验表驱动（type 非法 / source 空 /
grep 超长 / tail 越界 / agents 形态）、目标筛选与 skipped 归因（离线 / 无 capability /
不存在 id）、扇出成功（注册 2 个假 agent 各返结果）、单 agent 失败降级（整体仍 200）、
在途上限 429；web `npx tsc --noEmit` 零错误 + `pnpm build` + 表单交互手验。

### M3 清单

- [x] server：`api_logs_search.go`（筛选 / 扇出 / 降级 / 并发闸）+ serveAPI 接入
- [x] web：跨机检索页（表单 / 分组卡片 / 高亮）
- [x] 测试 + 文档收尾（本清单勾选）+ todo.md 同步

✅ M3 完成（2026-09-19）：server 6 测试（校验 10 形态 + 405 / 扇出聚合与排序 /
no-logs 跳过双形态 / 单机失败降级 / 全目标不可用 200 / 在途闸门 429）全绿；
web tsc 零新增错误 + build 过。实现注记：单目标超时即 CallAgent 既有 30s 上限
（未另设 35s）；agent 断开即从注册表注销，offline 与 not-found 归因合并为
offline；结果按 agentId 排序保证输出稳定（registry.List 基于 map 遍历无序）。

## 不做（后续项）

- 推送式采集与服务端存储聚合（agent 常驻采集 → server 落库索引，或 Loki 对接）：
  独立量级；M3 联邦检索已闭环跨机排障，仅当出现历史趋势检索 / 合规留存类需求
  时再立项；
- 日志级别结构化解析（journalctl -o json 的字段提取）：当前文本形态已够用，且
  docker 源无对应语义（M3 设计复确认）；
- `--all` 历史对象枚举、多 unit 联合查询、PCRE 过滤：按需后补。

## M1 清单

- [x] agent：`internal/agent/rpc/logs_provider.go`（探测/源枚举/查询/过滤/截断）
      + `logs` capability 追加 + 按能力注册
- [x] server：`api_logs.go`（3 端点 + 双端参数校验）+ serveAPI 接入
- [x] web：Workbench「日志」Tab（源选择/过滤栏/着色视图/状态行）
- [x] 测试：源枚举解析、query 各形态（tail/since/grep/truncated）、白名单校验、
      server 转发与错误透传
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M1 完成（2026-09-15）：agent 7 测试 + server 4 测试全绿；grep 命中词前端高亮与
agent contains 过滤同语义（大小写敏感）；Workbench「日志」Tab 与「文件」Tab 同模式内嵌，
无 logs capability 主机呈现类型禁用 + 提示条。

## 参考

- 内部：[cron-design.md](./cron-design.md)（Commander + server 转发模式同构）、
  [file-manager-design.md](./file-manager-design.md)（Workbench Tab 与文件日志边界）、
  `todo.md` P2 日志聚合查看条目
- 外部：[journalctl(1)](https://www.freedesktop.org/software/systemd/man/journalctl.html)
  （-o short-iso / --since 语义）、[docker logs](https://docs.docker.com/engine/reference/commandline/logs/)
  （--tail/--timestamps/--since）
