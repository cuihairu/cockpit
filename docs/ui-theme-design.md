# UI 主题与皮肤

Web 与移动端各有一套 UI 外观设置。本文是外观体系的实现基线：色值表以本文为准，
代码里的常量与本文不一致时以本文为准并同步修代码。

## 一、两个正交维度

外观由两个**互相独立**的维度组合而成，用户可任意搭配：

| 维度 | 取值来源 | 决定什么 | 字段 |
| --- | --- | --- | --- |
| **主题色**（强调色） | 预设色板 | 按钮、标签、选中态、链接等**强调元素**的色相 | `themeColor` |
| **主题皮肤**（中性色板） | 4 套皮肤预设 | 页面底、面板面、边框、文字的**中性色**与整体冷暖调 | `themeSkin` |

再加一个明暗档位（`light` / `dark` / `auto`，`auto` 跟随系统 `prefers-color-scheme`）。
组合空间 = 主题色 × 皮肤 × 档位，任一组合都成立。

设计上这么切分的原因：强调色换了不该动整屏底色，整屏底色换了也不该把按钮色相
一起改掉。历史上 Web 端把大量中性色硬编码在 `App.less` 里、并在
`html[data-theme='dark']` 下整块覆写，导致加第三种底色就得复制一份覆盖块；
现在色板数据化后，明暗与皮肤只表现为同一套 CSS 变量的不同取值。

## 二、皮肤清单

四套皮肤，每套含浅色 / 深色两份色板：

| key | 名称 | 定位 |
| --- | --- | --- |
| `default` | 默认 · 雾白 | 冷中性灰，历史默认观感 |
| `midnight` | 深邃 · 夜航 | 靛蓝夜幕，运维台气质 |
| `eyecare` | 护眼 · 米纸 | 暖米纸色，久看不刺眼 |
| `graphite` | 石墨 · 曜岩 | 零色相纯灰，最保守的底色 |

`default` 即改造前的观感，作为回退基准：任何 `themeSkin` 取到非法值时回退到
`default`（Web 端 `resolveThemeSkin`，移动端 `resolveAppSkin`）。

### 色值表

色板字段含义（两端同名）：

- `bg` 页面底色
- `surface` 卡片 / 面板面
- `surfaceAlt` 次级面（表头、条纹行）
- `border` 主边框
- `borderSoft` 更弱的分割线（表格行分隔）
- `text` 主文字
- `textMuted` 次级文字（列头、辅助说明）

#### default · 雾白

| 字段 | light | dark |
| --- | --- | --- |
| bg | `#f8fafc` | `#0c0e14` |
| surface | `#ffffff` | `#141620` |
| surfaceAlt | `#ffffff` | `#141620` |
| border | `#e2e8f0` | `rgba(255, 255, 255, 0.06)` |
| borderSoft | `#e2e8f0` | `rgba(255, 255, 255, 0.04)` |
| text | `#0f172a` | `#e2e8f0` |
| textMuted | `#64748b` | `#94a3b8` |
| inputBg | `#ffffff` | `#141620` |

#### midnight · 夜航

| 字段 | light | dark |
| --- | --- | --- |
| bg | `#f2f5fa` | `#070d1a` |
| surface | `#ffffff` | `#0e1729` |
| surfaceAlt | `#f7f9fc` | `#101b30` |
| border | `#dbe3ee` | `rgba(148, 180, 255, 0.14)` |
| borderSoft | `#e8edf5` | `rgba(148, 180, 255, 0.09)` |
| text | `#16233b` | `#dbe7ff` |
| textMuted | `#5f7192` | `#8296bd` |
| inputBg | `#ffffff` | `#0e1729` |

#### eyecare · 米纸

| 字段 | light | dark |
| --- | --- | --- |
| bg | `#f4efe4` | `#1a1712` |
| surface | `#fbf7ee` | `#241f19` |
| surfaceAlt | `#f7f2e7` | `#272119` |
| border | `#ded2ba` | `rgba(226, 205, 168, 0.18)` |
| borderSoft | `#e7ddc9` | `rgba(226, 205, 168, 0.11)` |
| text | `#3b342a` | `#e8dfcd` |
| textMuted | `#7b6f5b` | `#a3937a` |
| inputBg | `#fbf7ee` | `#241f19` |

#### graphite · 曜岩

| 字段 | light | dark |
| --- | --- | --- |
| bg | `#f4f4f5` | `#131313` |
| surface | `#ffffff` | `#1c1c1e` |
| surfaceAlt | `#fafafa` | `#1f1f21` |
| border | `#e4e4e7` | `rgba(255, 255, 255, 0.1)` |
| borderSoft | `#ededf0` | `rgba(255, 255, 255, 0.06)` |
| text | `#18181b` | `#ededf0` |
| textMuted | `#71717a` | `#9b9ba3` |
| inputBg | `#ffffff` | `#1c1c1e` |

### 顶栏 / 侧栏底色

顶栏与侧栏底色不由皮肤直接给出，而是由 `bg` 与 `surface` 按档位推导
（`chromeColor`）：浅色档取 `surface`（白底面板浮在灰底上，有层次），深色档取
`bg`（整屏一色，避免深色下的双重边框感）。

### hover 浓淡

hover 态底色是**主题色**按皮肤给定的透明度叠加（`hoverAlpha`，Web 端每套皮肤
light/dark 各有取值 0.03~0.08）。色相归主题色，浓淡归皮肤——所以暖色皮肤下的
hover 不会因为底色偏黄而显脏。

## 三、Web 端实现

### CSS 变量

`web/src/contexts/SettingsContext.tsx` 的 effect 按 `themeSkin` + 明暗档位 +
`themeColor` 计算变量全集写到 `<html>` 的行内 style，并同步
`data-skin="<key>"` 供调试与选择器使用：

| 变量 | 来源 |
| --- | --- |
| `--cockpit-bg` | 色板 `bg` |
| `--cockpit-surface` | 色板 `surface` |
| `--cockpit-surface-alt` | 色板 `surfaceAlt` |
| `--cockpit-border` | 色板 `border` |
| `--cockpit-border-soft` | 色板 `borderSoft` |
| `--cockpit-text` | 色板 `text` |
| `--cockpit-text-muted` | 色板 `textMuted` |
| `--cockpit-input-bg` | 色板 `inputBg` |
| `--cockpit-chrome` | `chromeColor`（顶栏/侧栏） |
| `--cockpit-hover` | 主题色 + `hoverAlpha` |
| `--cockpit-primary` | 主题色原值 |
| `--cockpit-primary-hover` | 深色档 `lightenColor(primary, .18)`，浅色档 `darkenColor(primary, .18)` |
| `--cockpit-primary-strong` | 深色档 `lightenColor(primary, .42)`，浅色档 `darkenColor(primary, .24)` |
| `--cockpit-primary-soft` | `withAlpha(primary, .10)` |
| `--cockpit-primary-faint` | `withAlpha(primary, .06)` |
| `--cockpit-primary-border` | `withAlpha(primary, .20)` |

强调色派生量在**运行时**由主题色 hex 计算，色相始终跟随主题色预设——改造前菜单、
标签、按钮上硬编码了蔷薇红，切主题色时不跟着变。

`web/src/App.less` 里原先写死的中性色全部改为 `var(--cockpit-*, 兜底色)` 引用，
兜底值即改造前的硬编码值，因此变量未就绪（首帧、SSR、测试环境）时页面观感不变。
`html[data-theme='dark']` 的整块覆盖已删除，一套规则服务所有皮肤 × 档位；只剩语义色
（success / warning / error）在深色下的提亮修正块。

### 组件库 token

`web/src/App.tsx` 的 `ConfigProvider` token 走同一份数据
（`resolveSkinTokens`），保证 antd 组件的颜色与手写 CSS 同源：`colorBgLayout`
取 `bg`、`colorBgContainer` 取 `surface`、`colorFillAlter` 取 `surfaceAlt`、
`colorBorder` / `colorBorderSecondary`、`colorText` / `colorTextSecondary`、
`Layout` 的 `headerBg` / `siderBg` / `bodyBg` 取 `chrome`。

### 设置页

`web/src/pages/Settings/GeneralSettings.tsx` 里：

- 「主题」Select 选主题色（标签保持为「主题」不变）
- 「主题皮肤」由 `web/src/components/ThemeSkinCards` 渲染成卡片组：每张卡的缩略图
  **用该皮肤自身色板内联绘制**（底 / 面板 / 边框 / 文字四色块），跟随当前明暗档位；
  整组是 `role="radiogroup"`，卡片是 `role="radio"`。

皮肤卡片点选**立即生效**（直接写 context，不经表单保存），因为皮肤是纯前端口径、
即时可见的效果，让用户为了看效果去点「保存」没有意义。为防保存其它设置时被表单
里的旧值回退，`useSettings.saveSettings` 用 `values.themeSkin ?? settings.themeSkin`。

### 持久化

Web 端设置整体存 localStorage（沿用既有 settings 通道），`themeSkin` 是
`SettingsContext` 归一化的一等字段；刷新后由 effect 重放，无需额外接口。

## 四、移动端实现

- `mobile/lib/theme/app_theme.dart`：`AppSkin` / `SkinPalette` 与四套皮肤同名同值，
  `buildAppTheme(skinKey, brightness)` 产出 `ThemeData`，深色底色取自 `dark` 色板。
- `mobile/lib/main.dart`：`MaterialApp` 的 `theme` / `darkTheme` / `themeMode` 全部
  接设置状态。
- `mobile/lib/pages/settings_page.dart`：设置页「主题外观」入口，弹出
  `_AppearanceSheet` 分「明暗档位」与「皮肤」两段；皮肤行用 `_SkinPreview` 画同款缩略图。
- `mobile/lib/state/settings.dart`：`themeMode`（`system` / `light` / `dark`）/
  `themeSkin` 字段 + `setThemeMode` / `setThemeSkin`，SharedPreferences 键
  `theme_mode` / `theme_skin`。存量安装无这两个键，缺省「跟随系统 + 默认皮肤」。

注意 Flutter 3.47 的两个坑：`ColorScheme.fromSeed` 产出的 `primary` 是 tonal 变体，
必须显式 `copyWith(primary: cockpitAccent, ...)` 钉回强调色；`RadioListTile` 的
`groupValue` / `onChanged` 已弃用，单选组要用 `RadioGroup<T>` 包裹。

## 五、两端口径一致性

「同步」的含义是**口径一致**，不是共享同一份存储：

- 皮肤 key、名称、说明、以及色板全部色值两端完全相同（本文第二节即共同基线），
  在任意一端切皮肤，另一端选同一个 key 得到同一套颜色。
- 服务端没有 UI 设置的持久化通道，因此两端各自存本地：Web 端 localStorage，
  移动端 SharedPreferences。不存在「改一处、两端同时变」的实时下发。
- **已知差异**：移动端暂不支持自定义强调色，强调色固定为 Web 默认主题色「蔷薇红」
  `#e11d8f`（`mobile/lib/theme/app_theme.dart` 的 `cockpitAccent`）。Web 端切主题色
  不会影响移动端。补齐移动端主题色选择后，此处需要同步更新。

## 六、新增一套皮肤的流程

1. 在 `web/src/theme/themeSkins.ts` 的 `THEME_SKINS` 与
   `mobile/lib/theme/app_theme.dart` 的 `appSkins` 里加同名同值条目。
2. 在本文第二节补色值表。
3. 跑两端单测（`web/src/theme/themeSkins.test.ts`、
   `mobile/test/app_theme_test.dart`）确认回退与解析行为未变。

不需要改 `App.less`、不需要加 `data-theme` 覆盖块、不需要动组件代码——新皮肤
自动出现在两端设置页的可选项里，并自动获得缩略预览。

## 七、相关文件

| 文件 | 职责 |
| --- | --- |
| `web/src/theme/themeSkins.ts` | 皮肤数据层、颜色工具、CSS 变量与 antd token 生成 |
| `web/src/contexts/SettingsContext.tsx` | 设置归一化、`data-skin`、CSS 变量写入 |
| `web/src/App.less` | 变量引用（不再按明暗覆写中性色） |
| `web/src/App.tsx` | `ConfigProvider` token 接皮肤数据 |
| `web/src/components/ThemeSkinCards/index.tsx` | 设置页皮肤卡片 + 缩略预览 |
| `web/src/pages/Settings/GeneralSettings.tsx` | 设置页外观区块入口 |
| `mobile/lib/theme/app_theme.dart` | 移动端皮肤与 `ThemeData` 构建 |
| `mobile/lib/state/settings.dart` | 移动端设置状态与持久化 |
| `mobile/lib/pages/settings_page.dart` | 移动端外观面板 |