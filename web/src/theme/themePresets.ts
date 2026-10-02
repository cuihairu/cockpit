// 主题色预设：antd colorPrimary、内联 logo 填色、App.less 的
// --cockpit-primary 三处共用同一份色值，保证全局一致。
export interface ThemePreset {
  key: string
  name: string
  color: string
}

export const THEME_PRESETS: ThemePreset[] = [
  { key: 'rose', name: '蔷薇红', color: '#e11d8f' },
  { key: 'blue', name: '远峰蓝', color: '#1677ff' },
  { key: 'cyan', name: '青碧', color: '#13c2c2' },
  { key: 'green', name: '翠绿', color: '#16a34a' },
  { key: 'purple', name: '罗兰紫', color: '#722ed1' },
  { key: 'orange', name: '落日橙', color: '#fa8c16' },
]

export const DEFAULT_THEME_COLOR = THEME_PRESETS[0].key

// 非法 key 回退默认（蔷薇红，即历史硬编码 #e11d8f，存量观感不变）
export const resolveThemePreset = (key?: string | null): ThemePreset =>
  THEME_PRESETS.find((p) => p.key === key) ?? THEME_PRESETS[0]
