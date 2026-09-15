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

## 不做（后续项）

- 断点续传/分块上传大文件（>10MB）：`file.write` append 通道已留好，UI 后补；
- 文本搜索/目录大小统计/回收站；
- chmod/chown/权限编辑：风险与 UI 复杂度都高，等真实需求；
- 文件预览增强（图片/二进制 hex）：编辑器先覆盖文本主场景。

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
