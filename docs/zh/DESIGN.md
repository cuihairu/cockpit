# Cockpit UI 设计基线

> 触发：`docs/review/2026-10-31-ui-audit-impeccable.md` T7（无 DESIGN.md / PRODUCT.md，审核无设计基准可比对）。
> 本文是**后续 UI 审核的比对基线**，给出色板清单本身没有写明的颜色*用法*与*对比度*规则。
> 权威色板值在 `docs/ui-theme-design.md`（皮肤 + 强调色）；本文不复抄该表，只引用并补充规则。

## 1. 适用范围

- 面：`web/`（React 19 + AntD 5 + ProComponents）。移动端沿用同一套皮肤数据（`mobile/lib/theme/app_theme.dart`），但本文的对比度规则针对 Web 面。
- 所有比值的基准：WCAG 2.x 相对亮度；下文「AA」指 4.5:1（正文）、3:1（大字号 ≥ 28px，或 ≥ 18.66px 粗体）。

## 2. 色板

- **强调色 / 主题色**：`web/src/theme/themePresets.ts` 的 6 个预设（`rose #e11d8f`、`blue #1677ff`、`cyan #13c2c2`、`green #16a34a`、`purple #722ed1`、`orange #fa8c16`），默认 = 蔷薇红 `#e11d8f`。通过 `--cockpit-primary*` 与 antd `colorPrimary` token 驱动按钮/标签/选中态/链接。
- **中性皮肤**：4 套皮肤 × 明暗（`default`、`midnight`、`eyecare`、`graphite`），字段 `bg / surface / surfaceAlt / border / borderSoft / text / textMuted / inputBg`。完整取值表：`docs/ui-theme-design.md` §2。默认 = `default`（雾白）。
- **语义状态色**（成功 / 警告 / 错误）：`web/src/App.less` 声明 `@success-color #22c55e`、`@warning-color #f59e0b`、`@error-color #ef4444`；不少组件改用 antd 默认值 `#52c41a` / `#faad14` / `#ff4d4f`。仪表盘统计数字用 `#00b42a`（绿）、`#ff7d00`（橙）、`#13c2c2`（青）。

## 3. 文字色规范

| 角色 | Token / 来源 | 规则 |
| --- | --- | --- |
| 主要文字 | 皮肤 `text`（如 `#0f172a`） | 在其 `surface` 上 ≥ 4.5:1；绝不低于此 |
| 次要文字 | 皮肤 `textMuted`（如 `#64748b`） | 在其 `surface` 上**≥ 4.5:1**——列头、辅助说明、统计标签、时间戳。审核的 F3/F4 修复已把若干处统一到 `#667085`（在 `#fff` 上 4.97:1）；该 token 即基线 |
| 占位色 | antd `colorTextPlaceholder` = `skin.textMuted` | **≥ 4.5:1**——占位符常常是字段唯一的标识，故**不适用** WCAG 豁免（审核 T1）。AntD 默认 `rgba(0,0,0,0.25)` 只有 1.84:1，不得使用 |

中性文字不变量（永不放松）：

- 任何文字或占位符在其自身 `surface` 上都不得低于 4.5:1（禁用控件是唯一的 WCAG 豁免，即便如此，作为字段标识的占位符也不豁免）。
- 次要文字比主要文字浅一档，但永不低于 AA。`#86909c`（3.24:1）与 `#999`（2.85:1）已退役。

## 4. 语义色规范

状态色有**双重角色**，对比度要求取决于角色：

1. **填充 / 图标 / 标签 / 徽标**（语义色作为色块、图标或圆点）：豁免文字 AA 规则——它们是图形对象。
2. **浅色面上的文字**：原始语义色通常*低于* AA，须改用**同色相更深变体**满足 ≥ 4.5:1（小字）/ ≥ 3:1（大字）。

在用颜色在 `#fff` 上的原始对比度（除品牌粉勉强外全部不达 4.5:1）：

| 颜色 | 用途 | 在 `#fff` 上 |
| --- | --- | --- |
| `#e11d8f` | 品牌粉（默认强调色） | 4.40:1 |
| `#1677ff` | 品牌蓝 | 4.10:1 |
| `#d48806` | 金色标签「内置」 | 2.87:1 |
| `#00b42a` | 仪表盘统计绿 | 2.78:1 |
| `#13c2c2` | 仪表盘统计青 | 2.21:1 |
| `#ff7d00` | 仪表盘统计橙 | 2.57:1 |
| `#ff4d4f` | 错误 /「清理离线 agent」 | 3.27:1 |
| `#52c41a` | antd 成功 | 2.27:1 |
| `#faad14` | antd 警告 | 1.90:1 |
| `#ef4444` | `@error-color` | 3.76:1 |

**角色 2 的规则**：选同色相更深变体。已验证在 `#fff` 上达标的候选：`#c41d7f`（粉，5.48:1）、`#0958d9`（蓝，6.16:1）、`#cf1322`（红，5.57:1）、`#d4380d`（橙，4.80:1）、`#ad6800`（金，4.41:1）。绿/青需要比这些更深的变体；具体绿/青文字取值留给 T3 设计决策。

> **开放决策**：审核 T3（品牌色/语义色在白底上的文字对比度）待拍板——把上述更深文字变体铺到受影响的位置是独立的设计变更，不在本次落地。本文只确立*规则*；原始色保留作为填充/图标色集。

## 5. 触控目标

- WCAG 2.5.8 最小 **24 × 24px**。审核 T6 发现若干 antd 小控件低于此值（42×21 按钮、1×15 分隔符、26×21 标签页、21px 数字输入）——目前仅作信息记录；主要交互控件应达到 ≥ 24px 高（`size` / `padding`），待触控成为主要目标后确认。

## 6. 布局

- 390px 视口下不得出现页面级横向溢出（桌面 1440px 同样）。密集表格须设置 `scroll` 属性 `x: 'max-content'`，页内工具条设 `wrap`（审核 F1/F2/T5）。
- 每个交互控件都需要可访问名称（审核 F6：头部搜索按钮）。

## 7. 审核清单（如何重新审核）

1. 对比度：所有文字/占位符在其 `surface` 上 ≥ 4.5:1（大字 ≥ 3:1）——分别在 1440×900 与 390×844、明暗两种下扫描。
2. 溢出：390px 下全部 22 条路由 `scrollWidth <= viewport`，包括加载态。
3. 命名：无未命名交互控件（按钮、开关、数字输入）。
4. 触控：主要操作 ≥ 24px 高。

## 8. 相关文件

| 文件 | 职责 |
| --- | --- |
| `docs/ui-theme-design.md` | 权威色板（皮肤、强调色、CSS var / token 接线） |
| `web/src/theme/themePresets.ts` | 强调色预设 |
| `web/src/theme/themeSkins.ts` | 皮肤数据 + token/CSS-var 生成 |
| `web/src/App.less` | 中性 var 引用 + `@success/warning/error-color` |
| `web/src/App.tsx` | ConfigProvider token（`colorTextPlaceholder` 等） |
| `docs/review/2026-10-31-ui-audit-impeccable.md` | 本文所回应的审核 |
