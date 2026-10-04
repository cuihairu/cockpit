# Windows Agent 打包设计（setup.exe + 服务化 + 图标全链）

> 状态：已落地。产物 `cockpit-agent-setup-<version>.exe` 进 nightly release（含
> `.sha256`），真 Windows 走查在 nightly 的 **Build Agent Installer** job 内完成。
> 需求来源：用户催办（B1，见仓库 `BUGS.md`）。

## 目标与形态

Windows 上的 agent 交付要满足三件事，缺一就是"半成品"：

1. **双击即用的安装包**：装目录、开始菜单、桌面快捷方式、卸载器
2. **装完即用（开机自启）**：注册为 Windows 服务，Automatic + 崩溃自动重启
3. **有图标**：exe 资源段、快捷方式、安装器、控制面板卸载条目同一枚图标

选择 Inno Setup（而非 MSI）：脚本化、可参数化静默装机、自定义安装页（Server 地址
输入）、Windows 服务任务用 agent 自身子命令收口。构建脚本见
[`packaging/windows/agent.iss`](https://github.com/cuihairu/cockpit/blob/main/packaging/windows/agent.iss)。

## 图标全链（SVG logo → ico → exe 资源）

`scripts/gen-windows-icon.sh` 一条命令再生全链，产物全部入库（构建不依赖 CI 内的
光栅化工具链）：

```text
web/public/logo.svg
  └─ magick 7 尺寸光栅化（16/24/32/48/64/128/256）→ packaging/windows/agent.ico
       ├─ Inno SetupIconFile / UninstallDisplayIcon → 安装器 + 卸载条目图标
       ├─ [Icons] 快捷方式图标（引用 exe 内嵌图标）
       └─ rsrc → cmd/cockpit-agent/rsrc_windows_{amd64,arm64}.syso
            └─ go build windows/amd64|arm64 自动链接进 PE .rsrc 段
```

关键约束：

- syso 必须命名 `<name>_<GOOS>_<GOARCH>.syso` 才会被 `go build` 拾取，且**只影响
  windows 目标**——linux/darwin 构建直接忽略，CI 不需要任何额外开关。
- ico 是多尺寸 PNG 容器（ImageMagick 直接拼 PNG，不走 BMP 编码，透明边缘无损）。
- 本地可用 `debug/pe` 断言 `.rsrc` 段的 RT_ICON/RT_GROUP_ICON 目录；视觉级验证
  （`ExtractAssociatedIcon`）交给 nightly 的真 Windows 走查。

## 服务化（agent 内置 SCM 支持）

服务注册、启动、停止、注销全部收口到 agent 自身子命令：

```text
cockpit-agent service install [-server ...]   # 注册/升级（Automatic + failure actions）+ 启动
cockpit-agent service uninstall              # 停止并注销
cockpit-agent service start|stop|status
cockpit-agent service run                     # SCM 调用入口（勿手动执行）
```

服务名/显示名/描述/失败重启策略（5s/10s/20s，24h 计数重置）与既有 `install.ps1`
注册口径逐字一致——两条安装路径互相识别、升级时互相覆盖。

### 为什么必须自己做服务态

Windows 上把**控制台程序**直接注册成服务（`New-Service` 指向 `cockpit-agent start`）
必然坏：进程不调用 `StartServiceCtrlDispatcher`，SCM 等 30 秒后判定无响应并杀掉，
服务永远起不来。这是 `install.ps1` 的存量形态，本设计一并修掉：

- `handleStart` 顶部做 SCM 上下文识别（`svc.IsWindowsService`）：在服务上下文里
  自动转入服务态，存量已注册的直挂服务在下一次重启后即自愈，无需用户干预。
- 控制台运行该识别恒为 false，行为完全不变（现有信号路径语义不动）。

### 退出语义：进程内看护，不靠 SCM 重启额度

agent 连不上 Server 时 `Start()` 返回错误。若沿用"进程退出、让 SCM 重启"的常规
做法，SCM 的失败重启额度只有 3 次（5s/10s/20s），开机时网络未就绪 / DNS 未生效 /
服务端临时下线就会把服务彻底耗死。因此服务态在**进程内**重连：

- 连接失败 → 记日志、15s 后重试，服务保持 `Running`（状态真实：进程确实活着）
- 崩溃/硬杀 → 仍由注册时的 failure actions 兜底
- SCM `Stop`/`Shutdown` → 关闭 stop 通道，`StartCmd.RunService` 触发 `a.Stop()`
  优雅退出（连接断开、注册表残留清理由 agent 内部完成）

`StartCmd.RunService(stop)` 与 `Run()` 的差别只在退出信号来源：systemd/容器路径
仍走信号分支（`Run`），服务路径走 stop 通道分支。

## 安装器结构

| 段 | 内容 |
|---|---|
| `[Setup]` | 固定 AppId（重复安装=升级合并）、`{autopf}\Cockpit Agent`、`PrivilegesRequired=admin`、`x64compatible` 64 位模式、`{param:server}`（`/SERVER=` 静默开关，Inno 原生常量）、`CloseApplications`+`RestartApplications`（升级时先停后启） |
| `[Tasks]` | `service`（注册服务并开机自启，默认勾选）、`desktopicon`（桌面快捷方式） |
| `[Code]` | 自定义输入页收集 Server WebSocket 地址；未勾服务任务时跳过该页；静默安装从 `/SERVER=` 取；两处都为空则显式失败（不让 `service install` 拿空 `-server` 去校验后失败） |
| `[Files]` / `[Icons]` | 载荷为 CI 暂存的 `cockpit-agent.exe`；开始菜单含卸载入口 |
| `[Run]` | `service install -server ...`（`Tasks: service`） |
| `[UninstallRun]` | 先于文件删除执行 `service uninstall`（stop + 注销） |
| `[UninstallDelete]` | 清 `%ProgramData%\CockpitAgent`（服务日志 + `config.env`，不在 `{app}` 内） |

## 与 `install.ps1` 的关系

`install.ps1`（无 GUI / 批量装机）能力对齐安装器，服务注册改为调用
`cockpit-agent service install`：不再自己拼 `New-Service` + `sc failure`，也不再
`Remove-Service` 后重建（agent 内部走升级路径：停服务 → 刷新 `BinaryPathName` →
重启）。升级脚本与升级安装包可以互相覆盖同一服务条目。

## 验收（nightly 真 Windows 走查）

`Build Agent Installer` job 在 `windows-latest` 上：静默装机 → 断言
服务 `Automatic` + `Running` → `ExtractAssociatedIcon` 抽图标存证 → 开始菜单/
桌面快捷方式存在 → 桌面截图留档 → 静默卸载 → 断言 exe/服务/快捷方式全清，
打印 `WALKTHROUGH_OK`。图标 PNG 与桌面截图作为 artifact 上传（`walkthrough-icon.png`
/ `walkthrough-desktop.png`），安装器本体与 `.sha256` 进 release 资产。

代码侧可在 linux 覆盖的部分：非 Windows 平台 `service` 子命令的平台指引分支
（`cmd/cockpit-agent/cov_service_test.go`）、`RunService` 的参数校验透传与
"stop 先于 start 时优雅退出"语义（`internal/agent/cov_runservice_test.go`）；
SCM 状态机本体只能由真 Windows 走查覆盖。
