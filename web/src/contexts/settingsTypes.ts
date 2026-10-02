export type UITheme = 'light' | 'dark' | 'auto'

export interface UISettings {
  siteName: string
  refreshInterval: number
  enableNotifications: boolean
  theme: UITheme
  // 主题色预设 key（见 theme/themePresets.ts），非法值回退默认
  themeColor: string
  compactMode: boolean
  showResourceCount: boolean
}

export interface SettingsContextType {
  settings: UISettings
  updateSettings: (patch: Partial<UISettings>) => void
  resolvedTheme: Exclude<UITheme, 'auto'>
}
