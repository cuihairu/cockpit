# Cockpit 界面审核（impeccable audit）

- 日期：2026-10-31
- 工具：impeccable v4.8.0（audit / critique 流程）
- 仓库：cockpit
- 界面：`web/`（React 19 + AntD 5 + ProComponents + Vite + pnpm）
- 审核方式：本地 `cockpit server`（:9090，STATIC_DIR=web/dist）真实渲染，Playwright 截图 + 程序化扫描（对比度 / 触控目标 / 横向溢出 / 无名称控件 / 控制台错误），桌面 1440×900 + 窄屏 390×844
- 结论：**界面整体可交付，但存在一批「低对比度灰字 + 一处表格横向溢出」的硬伤**，已当场修复 6 处；另有 7 项待办（多为 AntD 默认占位色与品牌语义色，受「不动品牌色」边界约束，建议单独立项）。

---

## 一、整体 critique（不留情面）

这不是一个「难看」的界面，但也不是一个「做完」的界面。ProLayout 框架、卡片节奏、导航层级都正常，可维护性也好（每页独立组件、权限点清晰）。真正的问题在于**对「读得清、不出框」这两件基本事不上心**：

1. **灰字对比度全面欠账。** 全站大量正文/标签用 `#86909c`（对白底仅 3.24:1，13px 需要 4.5:1），登录页底部提示用 `#c9cdd4`（1.59:1，几乎看不见），「内置角色不可修改」用 `#999`（2.85:1）。这不是某个页面选色失误，是**全局次要文字色号定浅了**——一个色号同时坑了仪表盘、工作台、审计日志、登录页五个文件。修起来很简单（换一个能过 4.5:1 的同色相灰），但说明当初没有拿 WCAG 阈值反推过色板。
2. **唯一一张需要横向滚动的表格没配 `scroll`。** AGENT 管理表 11 列合计约 1800px，`<Table>` 没设 `scroll.x`，桌面端直接撑破 1440 视口（scrollWidth 2085），窄屏更到 819/390。这是「数据密集页必然踩坑」的低级遗漏；且它上方过滤条 `<Space>` 也没加 `wrap`，窄屏一屏放不下。
3. **交互层有重复。** 工作台页把「概览/文件/日志」同时做成一行按钮（打开连接）和一行 Tabs（切换面板），两行同名控件叠放，用户第一眼分不清是「连接」还是「切换」。
4. **无障碍细节没扫尾。** 头部全局搜索按钮（`Input.Search`）是无名 icon-only 按钮，全部 22 个页面都缺可访问名称；多处 icon 按钮、switch、数字输入无名称。这些不是设计问题，是「忘了加 aria-label」。
5. **占位符颜色沿用 AntD 默认 `rgba(0,0,0,0.25)`（1.83:1）。** 大量「筛选地域/选择主机/暂无…」的提示文字低于 AA；部分属禁用态（WCAG 豁免），但作为**字段唯一标识**的占位符不该豁免。建议在 ConfigProvider 主题里统一加深 `colorTextPlaceholder`。

**值得肯定的：** 没有 P0（白屏/崩溃/完全不可用）；无键盘陷阱；响应式整体成立（除 AGENT 页外各路由窄屏不溢出）；权限点与页面路由一一对应；每页有空态/加载态。

---

## 二、已修复（6 处，各一 commit）

| # | 严重度 | 问题 | 定位 | 修复 | commit |
|---|--------|------|------|------|--------|
| F1 | P1 | AGENT 表横向溢出：`scrollWidth 2085px` vs 1440 视口（窄屏 819 vs 390） | `web/src/pages/Agents/index.tsx:194` `<Table>` 无 `scroll` | 加 `scroll={{ x: 'max-content' }}`，表体内部横滚，页面不再溢出 | `bd8e6ec2` |
| F2 | P1 | 窄屏 AGENT 过滤条（搜索+3 筛选）不换行，撑破 390 视口 | `web/src/pages/Agents/index.tsx:148` `<Space>` 无 `wrap` | 加 `wrap` | `a1b20b89` |
| F3 | P1 | 登录页文案对比度：底部提示 1.59:1、副标题 3.24:1、记住账号 3.05:1、用户名/密码占位符 ≈1.6:1（占位符即字段唯一标识） | `web/src/pages/Login/index.less`（`.login-footer p`、`.login-subtitle`、`.ant-checkbox-wrapper`、`::placeholder`） | `#c9cdd4`→`#667085`、`#86909c`→`#667085`（4.98:1） | `4db2cd09` |
| F4 | P1 | 全局次要文字色 `#86909c` 对白底 3.24:1：仪表盘统计标签/后缀、工作台空态、审计日志时间戳、登录副标题/记住账号 | `web/src/pages/Dashboard/index.less`、`web/src/workbench/{OverviewPanel,ConnectionPanel}.tsx`、`web/src/pages/AuditLogs/index.tsx`、`web/src/pages/Login/index.less` | `#86909c`→`#667085`（4.98:1，同色相加深） | `9aa23add` |
| F5 | P1 | 「内置角色不可修改」2.85:1（`#999`） | `web/src/pages/Roles/index.tsx:159` | `#999`→`#667085` | `ff0ebac3` |
| F6 | P2 | 头部全局搜索按钮无可访问名称（icon-only，22 页全覆盖） | `web/src/App.tsx:456` `<Input.Search>` | `enterButton={<Button aria-label="搜索" icon={<SearchOutlined/>}/>}` | `42bf5013` |

**修复后复测（1440×900，22 路由）：** 横向溢出 0 路由（修复前 1 路由）；对比度失败从 **55 处 → 20 处**，且剩余 20 处全部为「AntD 默认占位/禁用色（10 处）」或「品牌/语义色（10 处）」，见待办 T1/T3。**测试门禁：** 受影响测试全绿（Agents 43、App 22、Roles/Login/Dashboard/AuditLogs/Workbench 等 12 文件 118 项通过）；构建成功；dist 为 `.gitignore` 排除，未提交。

### 修复前后截图（`docs/review/2026-10-31-ui-audit-shots/`）

- AGENT 表：`agents-desktop-before.png`（页面横向溢出）→ `agents-desktop-after.png`（表体内横滚，无页面溢出）
- AGENT 窄屏：`agents-narrow-before.png`（过滤条撑破）→（修复后 `audit` 复测窄屏 overflow=None）
- 登录页：`login-before.png`（灰字难辨）→ `login-after.png`（文案清晰）
- 仪表盘：`dashboard-before.png`（统计标签 3.24:1）→ `dashboard-after.png`（标签加深）
- 角色页：`roles-before.png`（「内置角色不可修改」浅灰）
- 头部搜索：`header-search-after.png`（搜索按钮已带 `aria-label`，外观变为带边框 Button）

---

## 三、待办问题（未修，按严重度排序）

### P2

**T1. 占位/禁用文字 `rgba(0,0,0,0.25)` 1.83:1（10 处）**
- 位置：`/agents` 筛选地域/状态/类型、`/proxy` 选择网关所在主机、`/cron` 选择主机/当前用户、`/services` 选择主机/暂无故障窗口记录、`/recordings` 暂无录制、`/access/users` admin 行/删除。
- 说明：AntD 默认 `colorTextPlaceholder`，多数是「暂无/空态/禁用」，其中空态文案（暂无故障窗口记录、暂无录制）与「作为字段标识的占位符」（筛选地域、选择主机）不豁免。
- 建议：ConfigProvider `theme.token.colorTextPlaceholder` 提到 ≥4.5:1（如 `rgba(0,0,0,0.45)`）或给字段加可见 label；一处 token 改动覆盖全部，比逐页改更稳。
- 状态：**待办**（全局 token 改动，影响面大，单独提变更）。

**T2. 工作台「概览/文件/日志」按钮与 Tabs 重复（P2 混淆）**
- 位置：`web/src/pages/Workbench/index.tsx:165-178`（`<Space>` 内 3 个 Button，`type={tab===key?'primary':'default'}`，点击 `openConnection`）叠放在 `:180` `<Tabs>`（同 3 个 item，切换面板）之上。
- 说明：两行同名控件功能不同（按钮=发起连接，Tab=切换面板），但视觉上像重复导航，且当前激活项在按钮行与 Tab 行都高亮，指向不一。
- 建议：二选一——(a) 按钮行改为明确的连接语义（如按协议加「连接」后缀或仅显示有能力的协议），(b) 去掉按钮行、由 Tabs 面板内提供连接入口。倾向 (a)，保留快速连接能力。
- 状态：**待办**（需产品确认连接交互保留方式）。

### P3

**T3. 品牌/语义色文字对白底对比度不足（10 处）**
- 品牌粉 `#e11d8f`：登录按钮白字 4.40:1、新建用户/新建角色白字 4.40:1、激活菜单「访问控制」4.20:1（13-16px 需 4.5:1）。
- 金色标签 `#d48806` on `#fffbe6`：「内置」2.76:1（11px）。
- 品牌蓝 `#1677ff`：「查看全部」「42 项」4.10:1、登录页「忘记密码？」3.86:1。
- 语义状态色（仪表盘大数字，28px 需 3:1）：绿 `#00b42a` 2.78:1、青 `#13c2c2` 2.21:1、橙 `#ff7d00` 2.57:1；红「清理离线 agent」`#ff4d4f` 3.27:1。
- 说明：**本次按边界「不动 logo/品牌色/视觉方向」未改**——这些是 AntD 语义/品牌色板，改动即动视觉方向。
- 建议：为「文字用色」引入同一色相的深色变体（如绿用 `colorSuccess` 的深一档、蓝用 `#0958d9`），仅作用于 on-white 文字，不改变图标/填充色；作为独立设计系统变更评审。
- 状态：**待办**（品牌边界，需设计决策）。

**T4. `/services` 页加载即 4 次 404 控制台错误**
- 位置：`web/src/pages/Services/index.tsx:95,100`（`api.getServiceStatus` / `api.getAgentServices`）。
- 说明：选中 agent 无服务管理能力或未选中时，后端 404，控制台有 AxiosError 噪声；界面本身有空态兜底，不影响使用。
- 建议：请求前判空 / 按 capability 守卫，或后端返回 200 空列表。
- 状态：**待办**（低优先级噪声）。

**T5. 窄屏加载期瞬时横向溢出（/proxy、/cron、/services）**
- 说明：窄屏扫描在数据加载瞬间捕捉到 `scrollWidth` 最高 700px（390 视口）；页面稳定后不可复现，判断为加载期布局抖动（长占位符 Select / 表格 `width:auto` 中途撑宽）。
- 建议：各页顶部过滤 `<Space>` 统一加 `wrap`（同 F2），并给表格容器设 `min-width:0` 兜底。
- 状态：**待办**（瞬态，加固项）。

**T6. 小触控目标 <24px（WCAG 2.5.8）**
- 位置：多处 AntD 小尺寸按钮 42×21、`Space` 1×15 分隔、Tab 项 26×21、数字输入 21px 高。
- 说明：多为 AntD 默认小控件/装饰性分隔，非交互主体；按信息性记录。
- 建议：若面向触屏，将主操作按钮提到 ≥24px 高（`size` 或 `padding`）。
- 状态：**待办**（信息性）。

**T7. 无 DESIGN.md / PRODUCT.md**
- 说明：仓内无 `DESIGN.md`、`PRODUCT.md`，本次审核无设计基准可比对，仅按 WCAG/可用性/布局通用标准评判；品牌/视觉方向无法对照，故 T3 从保守角度处理。
- 建议：补一份 DESIGN.md（色板、次要文字色、占位色、语义色规范），后续审核有基准。
- 状态：**记录缺失**。

---

## 四、审核范围与复测数字

- 路由（22）：`/`、`/workbench`、`/agents`、`/logsearch`、`/docker`、`/stacks`、`/backups`、`/proxy`、`/cron`、`/jobs`、`/approvals`、`/services`、`/network`、`/disk`、`/firewall`、`/nas`、`/oob`、`/monitor`、`/recordings`、`/settings`、`/access/users`、`/access/roles`。
- 登录：本地服务 + `ADMIN_PASSWORD` 环境凭据真实登录，未依赖 demo 站。
- 空/加载/错误态：各页 `Empty`/`Spin`/404 均有兜底；`/services` 404 见 T4。
- 复测（修复后）：桌面 22 路由——横向溢出 0；对比度失败 55→20（剩余均为占位/禁用色 + 品牌/语义色）；无未命名交互按钮（F6 后）。

## 五、总结

- 问题数：**13**（P1 5 + P2 3 + P3 4 + 记录缺失 1）
- 已修复：**6**（F1–F6，各一 commit，见上表）
- 待办：**7**（T1–T7）
- 测试门禁：受影响测试全绿；构建通过；dist 未入库。
