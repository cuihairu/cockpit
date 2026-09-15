# 移动端适配设计（M1：全局布局 + 关键页面）

> 2026-09-15 立项。对应 todo「移动端适配：关键页面（Dashboard/Monitor）
> 做移动端优化」。P2 最后一个独立功能条目。

## 痛点

手机访问面板是真实场景（出门在外看告警、临时重启一个容器），但当前
UI 只保证桌面可用：

- **mix 布局的顶部菜单在窄屏溢出**：16 个菜单项平铺 header，手机上
  换行/挤出视口；
- **Modal 默认 520px**：在 375px 宽的手机屏上直接超出视口；
- **表格无 `scroll.x`**：多列表格（如 Dashboard Agent 5 列）窄屏
  挤压换行成一团；
- **固定宽度控件**：Monitor 的 Agent 下拉 `width: 250` 固定值。

已有基础：Dashboard 统计卡 / Monitor 图表 / SystemInfoCard 栅格
已按 `xs/sm/md/lg` 断点写好；`App.less` 已有一个 768px media 块
（隐藏 header 搜索框、缩 stat-card）。

## 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D1 | 范围 | 全局布局层（影响所有页面）+ Dashboard + Monitor + 全局兜底 CSS；不做全站逐页精调 | todo 原文就是「关键页面」；全局层一次解决最大公约数（菜单/Modal/padding） |
| D2 | 窄屏判定 | `Grid.useBreakpoint()`，`isMobile = !screens.md`（< 768px）；布局切换必须 JS（`layout` prop），样式兜底继续用 media query | AntD 官方断点；与既有 768px media 块对齐 |
| D3 | 布局切换 | `layout = isMobile \|\| compactMode ? 'side' : 'mix'`——mix 的顶部菜单窄屏溢出，强制切 side；side 布局下 ProLayout 内建窄屏 Drawer（hamburger 抽屉菜单） | mix 的 header 菜单无溢出收纳机制；side + 抽屉是移动端标准形态 |
| D4 | Header 精简 | 搜索框已有 media 隐藏；「文档」按钮 < 768 只留图标（文字 span 加 class 隐藏）；用户名保留 | header 在窄屏只承担 logo + 通知 + 用户 |
| D5 | Modal 兜底 | 全局 CSS：`.ant-modal { max-width: calc(100vw - 16px) }` | 一行兜底所有现存与未来 Modal，不逐个改组件 |
| D6 | 表格策略 | AntD 无 `scroll.x` 时列内容自动换行不溢出（可接受兜底）；仅 Dashboard Agent 表（5 列）显式加 <span v-pre>`scroll={{ x: 640 }}`</span>（行内代码无 v-pre，JSX 花括号须手动包裹防 Vue 插值解析） | 全站补 scroll.x 是 M2 量级；M1 只修最挤的关键表 |
| D7 | Dashboard | 统计卡 `xs` 24 → 12（窄屏 2 列）；页头标题行 `flex-wrap`（刷新按钮不掉出视口） | 卡片是小号统计数字，2 列是手机标准密度 |
| D8 | Monitor | Agent 下拉固定 250px → 窄屏 `width: '100%'`；PageContainer extra 自身会 wrap | 其余栅格（图表 xs=24 lg=12、SystemInfoCard）已响应式 |
| D9 | 间距密度 | 窄屏 `page-container`/`dashboard-container` padding 12px、card body 16px | 手机上 24px 边距浪费屏宽 |
| D10 | 验证 | `pnpm build` + `pnpm lint`；真机 / DevTools 设备模拟验收留给使用者 | 本环境无浏览器实测手段，布局代码以断点逻辑正确性为保证 |

## 不做（后续版本）

- 全站每页 Table 补 `scroll.x`、列级 `responsive` 隐藏（M2 按需）；
- 独立移动端导航（底部 Tab Bar）——Drawer 够用；
- Workbench 终端/桌面的触屏交互优化（终端在手机上本就非目标场景）。

## M1 清单

- [x] App.tsx：useBreakpoint + 三态 layout + 文档按钮文字 class
- [x] App.less：768px media 块扩充（Modal 兜底 / 文档文字隐藏 / 间距）
- [x] Dashboard：统计卡 2 列 + Agent 表 scroll.x + 页头 wrap
- [x] Monitor：Agent 下拉窄屏自适应宽度
- [x] 验证：pnpm build + lint 通过
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M1 完成（2026-09-15）：`Grid.useBreakpoint()` 判定 < 768px，
`layout = isMobile || compactMode ? 'side' : 'mix'`（mix 顶部菜单窄屏
溢出 → side + ProLayout 内建抽屉）；768px media 块扩充 Modal
`max-width: calc(100vw - 16px)` 兜底、文档按钮只留图标、容器/卡片
间距收紧、表格 cell padding 8px；Dashboard 统计卡 xs=12（窄屏 2 列）
+ Agent 表 <span v-pre>`scroll={{ x: 640 }}`</span> + 页头 flex-wrap；Monitor Agent
下拉窄屏 100% 宽。Go 侧零改动。真机验收留给使用者（本环境无浏览器）。

## 参考

- 内部：`web/src/App.less`（既有 768px media 块）、Settings 的
  compactMode（同为布局形态切换先例）
- 外部：Ant Design Grid.useBreakpoint、ProLayout 响应式行为
