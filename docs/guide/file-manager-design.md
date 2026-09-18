# P1 方案设计：文件管理器（Agent 远程文件浏览 / 编辑 / 上传 / 下载）

> 2026-09-15。P1「文件管理器」首期设计。todo.md 原文：「通过 Agent 远程文件浏览/
> 上传/下载（Workbench 目前只有终端和桌面），支撑配置编辑与日志文件查看」。
> 本文只做设计决策与接口定义。

## 现状与痛点

| 痛点 | 现状 | 后果 |
|------|------|------|
| 改一个配置文件要开 SSH | Workbench 只有终端/桌面，改 nginx.conf 要 `vi` | 高频动作摩擦大，移动端基本不可用 |
| 看日志要敲命令 | `tail -f` / `less` 手工翻 | 没有搜索、没有高亮、没有下载 |
| 传文件要 scp | 本机 ↔ 服务器来回敲命令 | 忘记路径、权限问题的经典翻车点 |
| 危险操作无审计 | rm/mv 只留在 shell history | 出事无从追溯 |

## 架构总览

沿用备份 M1.5 验证过的通道：**分块 RPC base64 经出站 WebSocket，server 纯转发不落盘**
（文件管理器没有大文件中转需求，编辑/上传都是 KB–MB 级，下载走流式分块拉取）。

```
┌─ server ─────────────────────┐         ┌─ agent ─────────────────────┐
│ REST /api/agents/{id}/files  │ ─RPC─▶  │ file provider（新）          │
│ 参数校验 + 审计（变更类操作） │ ◀─RPC─  │ list/read/write/mkdir/      │
│ download 流式转发（不落盘）   │         │ delete/rename               │
└──────────────────────────────┘         └─────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 可见范围 | Agent 全文件系统自由导航，不设根目录白名单 | Agent 本有终端（等价 root 能力），白名单制造虚假安全感还挡住合法场景 |
| D2 | M1 能力边界 | 浏览 / 读 / 编辑保存 / 删除 / 重命名 / 新建目录 / 上传（≤10MB）/ 下载（不限大小）；不做 chmod/chown、搜索、目录大小统计 | 高频运维动作优先；权限修改风险大场景少，后补容易 |
| D3 | 传输通道 | `file.read` / `file.write` 分块 base64 RPC，复用出站 WebSocket | 与 `backup.read` 同构，协议零扩展，server 不落盘 |
| D4 | 读通道分工 | 浏览器**编辑器**走 `read`（≤1MB 文本，超出拒绝）；**下载**走 server 循环 read 256KB 流转发（无总大小限制，断开即停） | 编辑大文本卡 UI 无意义；下载流式天然支持 GB 级日志 |
| D5 | 写/上传 | `file.write {path, data, truncate}`：truncate=true 覆盖（编辑保存/新建），false 追加（上传分块续传）；单块 ≤1MB，客户端整文件 ≤10MB | M1 不做断点续传，10MB 覆盖配置/证书/小日志场景 |
| D6 | 删除 | `file.delete` 支持递归删目录；UI：文件 Popconfirm，**目录要求输入目录名确认**（GitHub 模式）；一律审计 | rm -rf 必须有摩擦 |
| D7 | 符号链接 | 列表标注 `isSymlink`+`target`；read/write/delete 拒绝直接作用于 symlink 路径；不跟随 | 与备份「不跟随」原则一致；symlink 穿越是最经典攻击面 |
| D8 | capability | 新增 `file` capability，**全平台注册**（Go 标准库，openwrt/busybox 通用） | 文件操作无平台差异；backup 是 Linux-only，file 不是 |
| D9 | 审计 | 写/删/重命名/新建目录/上传 记审计（ResourceFile）；浏览/读/下载不记 | 读操作量大无审计价值，变更必须可追溯 |
| D10 | 搜索语义 | `file.search {dir, query, caseSensitive?, maxResults?}`，query 纯文本 contains（非正则），默认大小写不敏感 | 与 logs grep 同纪律（防 ReDoS、双端行为一致可测）；运维搜配置常大小写混用，默认不敏感更实用 |
| D11 | 扫描边界 | 递归深度 ≤8；跳过 symlink（不跟随，D7 延伸）与非普通文件；二进制跳过（首 512B 含 NUL）；单文件 >1MB 跳过（与编辑器 read 上限一致）；文件总数 ≤5000；命中 ≤200 条即停；总超时 15s 返回已扫部分；排除 `.git`/`node_modules` 目录，其余隐藏目录照搜（`.ssh`/`.config` 可能正是目标） | 大目录树/大日志不拖垮 agent；每个上限都有 truncated/skipped 计数可解释 |
| D12 | server 端点 | `POST /api/agents/{id}/files/search` 纯转发，双端同规则校验（dir 路径 + query 非空 ≤256），不审计（浏览性质，D9 延伸） | 与既有文件端点同模式 |
| D13 | web 交互 | 工具栏「搜索」按钮 → Modal（当前目录为根 + 关键词 + 大小写开关）→ 结果列表（相对路径 + 行号 + 命中行高亮，行截断 200 字符）→ 点路径跳转所在目录 | 搜索天然以当前浏览位置为起点 |
| D14 | 图片预览 | 纯 web 实现，复用 `files/read` 分块端点（零 Go 改动）；扩展名白名单 jpg/jpeg/png/gif/webp/bmp/svg/ico/avif；>20MB 拒绝预览引导下载；一律 `<img>` 渲染（含 SVG） | 预览是浏览性质，走既有 read 端点无新攻击面；`<img>` 上下文的 SVG 处于 secure static 模式脚本不执行，天然隔离；MIME 按扩展名映射、不依赖内容嗅探；20MB 上限外的图片属罕见场景，下载本地看体验更好 |

## Agent 侧设计

### capability 与注册

- `agent.go` detectCapabilities 无条件追加 `file` capability（metadata 带 M1 支持的
  maxWrite=1MB / maxUpload=10MB 提示值）；
- `providers.go` 无条件 `RegisterProvider(rpc.NewFileProvider(rpc.FileConfig{}))`
  （无平台判断，与 backup 的 GOOS==linux 形成对照）。

### RPC 方法

| 方法 | 参数 | 返回 | 同步/异步 |
|------|------|------|-----------|
| `file.list` | `{dir}` | `{entries: [{name, size, mode, mtime, isDir, isSymlink, target}]}` | 同步 |
| `file.read` | `{path, offset, length}` | `{data(base64), size, eof}` | 同步分块 |
| `file.write` | `{path, data(base64), truncate}` | `{size}`（写入后文件总大小） | 同步 |
| `file.mkdir` | `{path}` | `{}` | 同步 |
| `file.delete` | `{path}` | `{}` | 同步 |
| `file.rename` | `{path, name}` | `{}`（同目录内改名） | 同步 |

- `file.list` 只返回目录直属条目（不递归），按目录优先 + 名称排序；`mode` 展示为
  八进制（如 0644）；symlink 条目用 Lstat 标注，`target` 为链接目标（只读展示）；
- `file.read` 单块上限 `fileReadChunkLimit = 1MB`，offset≥size 返回 eof=true 空数据
  （与 backup.read 完全同构）；只允许读**普通文件**（Lstat 后 Mode().IsRegular()）；
- `file.write`：truncate=true 时 O_CREATE|O_TRUNC；truncate=false 时 O_APPEND（且文件
  必须已存在）；目录不存在自动 MkdirAll 父目录（0700）；返回写入后文件总大小；
- `file.mkdir`：已存在报错（防误把已有目录当新建）；MkdirAll 支持一次建多级；
- `file.delete`：文件直接删；目录递归删（RemoveAll）；symlink 只删链接本身
  （Remove 不跟随）；path Clean 后为 `/` 拒绝；
- `file.rename`：`name` 只允许文件名（`^[^/\x00]{1,255}$`，不含路径分隔符），
  目标已存在拒绝（不静默覆盖）。

### 路径安全（双端防御，server 同规则）

- 所有 path：`filepath.Clean` 后必须 `filepath.IsAbs`，拒绝相对路径与 `..` 形态
  （Clean 已消解 `a/../b`，残留 `..` 开头即非绝对）；
- write/mkdir/delete 的 path 不得为 `/`（根目录本身不可写不可删）；
- rename 的 `name` 不含分隔符，目标路径 Join 后仍在原父目录内（结构性保证）；
- read/write 目标若是 symlink 直接拒绝（D7），杜绝经链接写穿到任意位置。

## Server 侧设计

### REST API（server/api_files.go，registerFilesAPI，JWT 保护）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/agents/{id}/files/list` | `{dir}` → 转发 file.list |
| POST | `/api/agents/{id}/files/read` | `{path, offset, length}` → 转发 file.read（编辑器专用） |
| POST | `/api/agents/{id}/files/write` | `{path, data, truncate}` → file.write + 审计 file_write |
| POST | `/api/agents/{id}/files/mkdir` | `{path}` + 审计 file_mkdir |
| POST | `/api/agents/{id}/files/delete` | `{path}` + 审计 file_delete |
| POST | `/api/agents/{id}/files/rename` | `{path, name}` + 审计 file_rename |
| GET  | `/api/agents/{id}/files/download?path=` | 循环 file.read（256KB 块）流式转发，attachment，断开即停，15 分钟总超时 |

- 所有路径参数 server 侧执行与 agent 相同的 Clean/IsAbs/根目录校验（双端防御）；
- 下载复用备份下载的流转发骨架（Content-Length 由首块 size 透传、nosniff、
  中途失败截断由 Content-Length 不匹配暴露）；
- 上传不经 server 暂存：浏览器 base64 后 POST write，server 原样转发（内存里
  一过即弃）；
- 审计：`ResourceFile = "file"`，动作 file_write / file_mkdir / file_delete /
  file_rename；detail 记 path（路径属操作信息可记，**文件内容从不入审计**）。

## Web UI 设计

- Workbench `WorkbenchTab` 增加 `'files'`，Tab「文件」（FolderOutlined），
  与 SSH/RDP/VNC 并列——文件管理天然按 agent 作用域，复用左侧 Agent 树；
- 新组件 `web/src/components/FileBrowser/index.tsx`：
  - **工具栏**：面包屑路径（可点击逐级回退 + 手输路径跳转）、刷新、新建目录、上传；
  - **列表** Table：名称（目录/文件图标 + symlink 标注）/ 大小 / 权限 / 修改时间 /
    操作（编辑[仅普通文件 ≤1MB]、下载、重命名、删除）；
  - **编辑 Modal**：read 全文 → TextArea（等宽字体）→ 保存（truncate write）；
  - **上传**：antd Upload 手动模式，beforeUpload 读 File → base64 →
    write（truncate=true），>10MB 前端拒绝；
  - **下载**：GET download → axios blob → createObjectURL（备份同款）；
  - **删除**：文件 Popconfirm；目录 Modal 输入目录名确认（D6）。

## M2：文本搜索（2026-09-18）

目录内递归文本搜索（D10-D13），排障场景定位「配置项写在哪个文件」。

### Agent 侧

- RPC `file.search`：`{dir, query, caseSensitive, maxResults}` → 
  `{matches: [{path, line, text}], truncated, scanned, skipped}`；
  - `filepath.WalkDir` 递归；相对路径相对 `dir` 呈现；行号从 1 起；
  - 匹配：`strings.Contains`（不敏感时 ToLower 双侧），命中行 `text` 截断 200 字符；
  - 每个 `matches` 命中即使停（D11），`truncated=true`；目录深于 8 / 文件数超 5000 /
    总超时 15s 同样置 truncated 提前收尾；skipped 计大文件与二进制；
- 路径校验与 list 同款（Clean/IsAbs/非根）；`query` 为空或 >256 字符拒绝。

### Server 侧

- `POST /api/agents/{id}/files/search`（api_files.go 分发）：双端同规则校验后转发
  `file.search`；agent 错误 502 透传；不审计。

### Web 侧

- FileBrowser 工具栏加「搜索」：Modal 内输入关键词（当前目录为根）+ 大小写开关；
- 结果列表：命中路径（点击跳转所在目录并关闭 Modal）+ 行号 + 命中行（关键词高亮）；
- 底部状态行：`N 处命中 · 已扫描 M 文件`，truncated/skipped>0 给提示 Tag。

**测试**：agent 临时目录树（命中与大小写开关 / 深度上限 / 二进制与大文件跳过 /
200 条截断 / 路径校验拒绝 / 空结果非错误）；server 转发与校验 400；web build。

## M2：图片预览（2026-09-18）

文件列表内直接预览图片（D14），免下载看截图/图标/证书二维码等场景。

### Web 侧（纯前端，零 Go 改动）

- **触发**：操作列「预览」按钮（EyeOutlined），仅对扩展名命中白名单
  （jpg/jpeg/png/gif/webp/bmp/svg/ico/avif，大小写不敏感）且非目录非 symlink
  的文件显示；
- **拉取**：`readFileChunk` 1MB 分块循环拼装，offset 按**已读字节数**推进
  （返回值 `size` 是文件总大小，不能当游标）；首块返回的 `size` >20MB 直接拒绝
  （提示走下载）；Modal 关闭置 cancelled 标志，循环立即中止不再发后续请求；
- **渲染**：按扩展名映射 MIME（image/jpeg 等，白名单内完备）→ `Blob` →
  `createObjectURL` → `<img>`；关闭与重新打开都 revoke 旧 URL；
- **安全**：SVG 一律走 `<img>`（浏览器 secure static 模式，脚本不执行），
  绝不 innerHTML；MIME 不信任 agent 内容嗅探（read 端点本就不返回 content-type）；
- **Modal**：标题为文件名；加载中 Spin；失败 Alert 兜底 + 引导下载；
  图片 `max-width:100% / max-height:60vh`、透明底纹深色底（透明 PNG 可见）；
  页脚显示自然分辨率（onLoad 读 naturalWidth×Height）与文件大小。

**测试**：纯 web 改动——`pnpm build`（tsc 零错误）+ 既有 Go 测试零回归。

## M3：分块上传 / 断点续传（2026-09-18）

上传从「≤10MB 单次 write」升级为「任意 ≤2GB 分块流式」：`file.write` 的
append 通道（M1 D5 预留）正式启用，agent 零改动。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D15 | 分块协议 | web 循环调既有 `files/write`（server 端点零改动，单块预检 1MB 原样）：单块 1MB（agent `fileWriteChunkLimit` 满额）；首块 `truncate=true` 建文件/清空旧内容，后续块 `truncate=false`（O_APPEND——agent 已有「append 目标必须存在」防碎片）；每块成功后校验返回 `size === 已传字节数`（agent 返回写入后总大小），不一致即终止（防交错/错位产出损坏文件） | 首块建文件的协议是 agent 既有设计（append 目标必须存在）；size 校验是顺序写入的自证 |
| D16 | 断点续传（会话内） | 块写入失败自动重试 ≤3 次：重试前 `files/list` 探测目标文件当前 size——等于已传字节数 → 从断点 append 续传；为 0/不存在 → 从头（首块重建）；其他值（并发写入）→ 终止报错。跨浏览器刷新不续传（v1，上传多为一次性动作） | 服务器文件 size 即续传游标，无需服务端会话状态；并发写检测保护用户不覆盖他人改动 |
| D17 | 大小分流 | ≤10MB 保持单块快速路径（一次 write，体验不变）；>10MB 走分块，上限 2GB（更大引导 scp/终端——base64 经 WebSocket RPC 的合理边界） | 小文件零开销，大文件解锁；上限值与下载「不限」对称（拉流便宜推流贵） |
| D18 | 审计分流 | server write 审计只记 `truncate=true`（新写入会话：编辑保存/分块首块/覆盖上传各一条）；`truncate=false`（分块续块）不记——否则 500MB 上传产生 500 条 file_write 刷屏，且续块无独立语义 | 会话级审计由首块代表；append 通道目前仅分块上传使用；终端等价 root 能力本就在审计面之外（D1 语义） |
| D19 | Web 交互 | 工具栏上传入口不变；>10MB 文件上传中在工具栏显示进度（`Progress` + 已传/总量）与「取消」；取消 = 中止后续块 + 删除半成品（`files/delete`，失败静默）+ 提示；上传完成刷新列表 | 进度就地可见；半成品不留垃圾；取消后重新上传首块 truncate 清空，语义自洽 |

### Server 侧

- `forwardFileRPC` write 分支：审计条件加 `truncate`（仅首块/覆盖写记
  `file_write`），其余逻辑不动。

### Web 侧

- `UPLOAD_MAX_BYTES` 语义改为单块路径阈值；新增分块上限 2GB；
- `uploadFiles` 分块循环（`File.slice` → base64 → write），进度/取消用
  token 递增（与图片预览同款）+ 半成品清理；失败探测续传（D16）。

**测试**：server——truncate=true 审计、truncate=false 不审计；web——tsc +
build；agent 零改动零回归。

## 不做（后续项）

- 跨刷新断点续传（服务端上传会话状态）；
- 目录大小统计/回收站；
- chmod/chown/权限编辑：风险与 UI 复杂度都高，等真实需求；
- 二进制 hex 预览：编辑器/图片预览先覆盖文本与图片主场景。

## M1 清单

- [x] agent：`internal/agent/rpc/file_provider.go`（list/read/write/mkdir/delete/rename
      + 路径安全 + symlink 拒绝）+ 无条件注册 + file capability（全平台）
- [x] server：`api_files.go`（7 个端点 + 双端路径校验 + 审计）+ serveAPI `/agents/` 分支
      接入 + audit 常量（ResourceFile + file_write/mkdir/delete/rename）
- [x] web：Workbench「文件」Tab + FileBrowser（面包屑/手输跳转/列表/编辑 ≤1MB/
      上传 ≤10MB/下载不限/重命名/删除——目录输入名字确认）
- [x] 测试：provider 单测（list 排序与 symlink 标注 / read 分块拼接 / write 覆盖与追加 /
      追加缺失文件拒绝（测试抓出实现偏差） / 穿越/symlink/根目录拒绝 / rename 校验 /
      symlink 删除只删链接本身）、server API 转发与校验、下载流转发逐字节比对、
      agent 离线 503
- [x] 文档收尾 + todo.md 同步

## 参考

- 内部：[backup-design.md](./backup-design.md) M1.5（file.read 分块与下载流转发骨架来源）、
  [stack-deploy-design.md](./stack-deploy-design.md)（Provider 注册模式）、
  [probe-enhance-design.md](./probe-enhance-design.md)（审计与通知模式）、`todo.md` P1 文件管理器条目
- 外部：[Filebrowser](https://github.com/filebrowser/filebrowser)（浏览交互范式）、
  OWASP Path Traversal（双端校验与 symlink 策略）
