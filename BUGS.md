# BUGS.md — 用户 bug 登记簿

> **流程制度（沿 falcon/memex 同款，2026-10-04 起）**：用户汇报的每条 bug
> **置顶登记于此**，优先修复，修完逐条回报。每轮工作汇报单列「用户 bug 清单
> 及状态」小节，漏一条视为不合格。不许出现「用户多次汇报一直被无视」的情况。

## 状态图例

`待处理` → `修复中` → `已修复（commit）` → `已到用户 builds`（修复进入用户可
获取的构建/发布后才算最终闭环）

## 活跃清单（置顶，newest first）

| # | bug 描述（用户口径） | 登记 | 状态 | 根因与修复 |
|---|---|---|---|---|
| B1 | Windows agent 要安装包（setup.exe/MSI：装目录/开始菜单/桌面快捷方式/卸载器，服务化注册+开机自启选项，进 nightly + SHA256）+ agent 二进制没图标（SVG logo→多尺寸 ico→exe 资源/安装包/快捷方式）——「提了很多天了，为啥没有做」 | 2026-10-04（**用户催办，登记簿查实此前漏排**） | 修复中 | 两项均从未排入 todo.md/工作流，属登记遗漏非技术阻塞。落地：① agent 补 x/sys/windows/svc 真服务支持（`service install/uninstall/start/stop/status/run` + `start` 在 SCM 上下文自动走服务态——兼修 install.ps1 存量 `New-Service` 直挂控制台进程 30 秒被 SCM 杀的隐患）；② logo.svg→多尺寸 .ico→rsrc .syso 进 exe；③ Inno Setup 安装器（服务/桌面快捷方式任务、服务器地址输入页）；④ nightly 增 windows-latest 打包 job（ISCC 出 setup.exe + .sha256）+ 真 Windows 安装-图标-服务-卸载走查截图 |
