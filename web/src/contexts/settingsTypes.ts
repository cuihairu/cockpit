export type UITheme = 'light' | 'dark' | 'auto'

export interface UISettings {
  siteName: string
  refreshInterval: number
  enableNotifications: boolean
  theme: UITheme
  // 主题色预设 key（见 theme/themePresets.ts），非法值回退默认
  themeColor: string
  // 主题皮肤 key（见 theme/themeSkins.ts，整屏中性色板预设），非法值回退默认
  // 可选：旧版本 localStorage 无此字段，normalizeSettings 补默认
  themeSkin?: string
  compactMode: boolean
  showResourceCount: boolean
}

export interface SettingsContextType {
  settings: UISettings
  updateSettings: (patch: Partial<UISettings>) => void
  resolvedTheme: Exclude<UITheme, 'auto'>
}
