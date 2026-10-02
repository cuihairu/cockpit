// 主题皮肤：整屏中性色板（底/面/边框/文字）预设，与 themePresets 的强调色正交。
//
// 定位差异：
// - 主题色（themePresets）决定按钮、标签、选中态等**强调元素**的色相
// - 皮肤（本文件）决定背景、面板、边框、文字的**中性色板**与整体冷暖调
//
// 两者正交组合：任意主题色 × 任意皮肤 × light/dark/auto 档位都成立。
// 皮肤值以 CSS 变量落到 <html>（SettingsContext 调 applySkinVars 写入），
// App.less 的 @bg-color/@surface-color/... 即这些变量的引用，因此换皮肤
// 只改一组变量，不重编样式表；App.tsx 的 antd token 取同一份 resolveSkinTokens，
// 保证组件库颜色与手绘 CSS 同步。
//
// 移动端同名同值（mobile/lib/theme/app_theme.dart），两端皮肤口径一致。

export type SkinMode = 'light' | 'dark'

export interface SkinPalette {
  /** 页面底色 */
  bg: string
  /** 卡片/面板面（浅色下兼作顶栏与侧栏底） */
  surface: string
  /** 次级面（表头、条纹行） */
  surfaceAlt: string
  /** 主边框 */
  border: string
  /** 更弱的分割线（表格行分隔） */
  borderSoft: string
  /** 主文字 */
  text: string
  /** 次级文字（列头、辅助说明） */
  textMuted: string
  /** 输入框底色 */
  inputBg: string
  /** 行/菜单 hover 态的强调色透明度（随主题色走，皮肤只定浓淡） */
  hoverAlpha: number
}

export interface ThemeSkin {
  key: string
  name: string
  /** 一句话说明，设置页卡片副标题用 */
  description: string
  light: SkinPalette
  dark: SkinPalette
}

export const THEME_SKINS: ThemeSkin[] = [
  {
    key: 'default',
    name: '默认 · 雾白',
    description: '冷中性灰，历史默认观感',
    light: {
      bg: '#f8fafc',
      surface: '#ffffff',
      surfaceAlt: '#ffffff',
      border: '#e2e8f0',
      borderSoft: '#e2e8f0',
      text: '#0f172a',
      textMuted: '#64748b',
      inputBg: '#ffffff',
      hoverAlpha: 0.03,
    },
    dark: {
      bg: '#0c0e14',
      surface: '#141620',
      surfaceAlt: '#141620',
      border: 'rgba(255, 255, 255, 0.06)',
      borderSoft: 'rgba(255, 255, 255, 0.04)',
      text: '#e2e8f0',
      textMuted: '#94a3b8',
      inputBg: '#141620',
      hoverAlpha: 0.05,
    },
  },
  {
    key: 'midnight',
    name: '深邃 · 夜航',
    description: '靛蓝夜幕，运维台气质',
    light: {
      bg: '#f2f5fa',
      surface: '#ffffff',
      surfaceAlt: '#f7f9fc',
      border: '#dbe3ee',
      borderSoft: '#e8edf5',
      text: '#16233b',
      textMuted: '#5f7192',
      inputBg: '#ffffff',
      hoverAlpha: 0.05,
    },
    dark: {
      bg: '#070d1a',
      surface: '#0e1729',
      surfaceAlt: '#101b30',
      border: 'rgba(148, 180, 255, 0.14)',
      borderSoft: 'rgba(148, 180, 255, 0.09)',
      text: '#dbe7ff',
      textMuted: '#8296bd',
      inputBg: '#0e1729',
      hoverAlpha: 0.08,
    },
  },
  {
    key: 'eyecare',
    name: '护眼 · 米纸',
    description: '暖米纸色，久看不刺眼',
    light: {
      bg: '#f4efe4',
      surface: '#fbf7ee',
      surfaceAlt: '#f7f2e7',
      border: '#ded2ba',
      borderSoft: '#e7ddc9',
      text: '#3b342a',
      textMuted: '#7b6f5b',
      inputBg: '#fbf7ee',
      hoverAlpha: 0.07,
    },
    dark: {
      bg: '#1a1712',
      surface: '#241f19',
      surfaceAlt: '#272119',
      border: 'rgba(226, 205, 168, 0.18)',
      borderSoft: 'rgba(226, 205, 168, 0.11)',
      text: '#e8dfcd',
      textMuted: '#a3937a',
      inputBg: '#241f19',
      hoverAlpha: 0.07,
    },
  },
  {
    key: 'graphite',
    name: '石墨 · 曜岩',
    description: '零色相纯灰，最保守的底色',
    light: {
      bg: '#f4f4f5',
      surface: '#ffffff',
      surfaceAlt: '#fafafa',
      border: '#e4e4e7',
      borderSoft: '#ededf0',
      text: '#18181b',
      textMuted: '#71717a',
      inputBg: '#ffffff',
      hoverAlpha: 0.04,
    },
    dark: {
      bg: '#131313',
      surface: '#1c1c1e',
      surfaceAlt: '#1f1f21',
      border: 'rgba(255, 255, 255, 0.1)',
      borderSoft: 'rgba(255, 255, 255, 0.06)',
      text: '#ededf0',
      textMuted: '#9b9ba3',
      inputBg: '#1c1c1e',
      hoverAlpha: 0.05,
    },
  },
]

export const DEFAULT_THEME_SKIN = THEME_SKINS[0].key

// 非法 key 回退默认（雾白，即历史观感）
export const resolveThemeSkin = (key?: string | null): ThemeSkin =>
  THEME_SKINS.find((s) => s.key === key) ?? THEME_SKINS[0]

const HEX_COLOR = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i

/** #rgb / #rrggbb → [r,g,b]；非法格式返回 null（调用方按原样透传） */
export const parseHexColor = (color: string): [number, number, number] | null => {
  const matched = HEX_COLOR.exec(color.trim())
  if (!matched) return null
  const body =
    matched[1].length === 3
      ? matched[1]
          .split('')
          .map((c) => c + c)
          .join('')
      : matched[1]
  return [
    parseInt(body.slice(0, 2), 16),
    parseInt(body.slice(2, 4), 16),
    parseInt(body.slice(4, 6), 16),
  ]
}

const clampByte = (n: number): string =>
  Math.max(0, Math.min(255, Math.round(n))).toString(16).padStart(2, '0')

/** 加透明度 → rgba()。已是 rgba()/色值函数时原样返回 */
export const withAlpha = (color: string, alpha: number): string => {
  const rgb = parseHexColor(color)
  return rgb ? `rgba(${rgb[0]}, ${rgb[1]}, ${rgb[2]}, ${alpha})` : color
}

/** 向黑压暗（0~1），用于浅色下的主色 hover/文字强调 */
export const darkenColor = (color: string, amount: number): string => {
  const rgb = parseHexColor(color)
  if (!rgb) return color
  return `#${rgb.map((c) => clampByte(c * (1 - amount))).join('')}`
}

/** 向白提亮（0~1），用于深色下的主色 hover/文字强调 */
export const lightenColor = (color: string, amount: number): string => {
  const rgb = parseHexColor(color)
  if (!rgb) return color
  return `#${rgb.map((c) => clampByte(c + (255 - c) * amount)).join('')}`
}

/** 当前档位下的中性色板（含顶栏/侧栏底色） */
export const resolveSkinPalette = (skin: ThemeSkin, mode: SkinMode): SkinPalette =>
  mode === 'dark' ? skin.dark : skin.light

/** 顶栏/侧栏底色：浅色下与面板同色（白底立体），深色下与页面底同色（整屏一色） */
export const chromeColor = (palette: SkinPalette, mode: SkinMode): string =>
  mode === 'dark' ? palette.bg : palette.surface

/**
 * 皮肤 + 档位 + 强调色 → 写入 <html> 的 CSS 变量全集。
 * App.less 通过 var(--cockpit-*, 兜底色) 引用，变量未就绪时回落历史硬编码值。
 */
export const buildSkinVars = (
  skin: ThemeSkin,
  mode: SkinMode,
  primary: string,
): Record<string, string> => {
  const palette = resolveSkinPalette(skin, mode)
  const onDark = mode === 'dark'
  return {
    '--cockpit-bg': palette.bg,
    '--cockpit-surface': palette.surface,
    '--cockpit-surface-alt': palette.surfaceAlt,
    '--cockpit-border': palette.border,
    '--cockpit-border-soft': palette.borderSoft,
    '--cockpit-text': palette.text,
    '--cockpit-text-muted': palette.textMuted,
    '--cockpit-input-bg': palette.inputBg,
    '--cockpit-chrome': chromeColor(palette, mode),
    '--cockpit-hover': withAlpha(primary, palette.hoverAlpha),
    // 强调色派生量：色相来自主题色预设，浓淡来自档位（深色下改提亮保可读）
    '--cockpit-primary': primary,
    '--cockpit-primary-hover': onDark ? lightenColor(primary, 0.18) : darkenColor(primary, 0.18),
    '--cockpit-primary-strong': onDark ? lightenColor(primary, 0.42) : darkenColor(primary, 0.24),
    '--cockpit-primary-soft': withAlpha(primary, 0.1),
    '--cockpit-primary-faint': withAlpha(primary, 0.06),
    '--cockpit-primary-border': withAlpha(primary, 0.2),
  }
}

export interface SkinTokens {
  /** 顶栏/侧栏底色 */
  chrome: string
  bg: string
  surface: string
  surfaceAlt: string
  border: string
  borderSoft: string
  text: string
  textMuted: string
  inputBg: string
  hover: string
}

/** antd ConfigProvider token：与 CSS 变量同源，避免组件库颜色与手绘样式脱节 */
export const resolveSkinTokens = (
  skinKey: string | undefined,
  mode: SkinMode,
  primary: string,
): SkinTokens => {
  const skin = resolveThemeSkin(skinKey)
  const palette = resolveSkinPalette(skin, mode)
  return {
    chrome: chromeColor(palette, mode),
    bg: palette.bg,
    surface: palette.surface,
    surfaceAlt: palette.surfaceAlt,
    border: palette.border,
    borderSoft: palette.borderSoft,
    text: palette.text,
    textMuted: palette.textMuted,
    inputBg: palette.inputBg,
    hover: withAlpha(primary, palette.hoverAlpha),
  }
}