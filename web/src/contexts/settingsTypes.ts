export type UITheme = 'light' | 'dark' | 'auto'

export interface UISettings {
  siteName: string
  refreshInterval: number
  enableNotifications: boolean
  theme: UITheme
  compactMode: boolean
  showResourceCount: boolean
}

export interface SettingsContextType {
  settings: UISettings
  updateSettings: (patch: Partial<UISettings>) => void
}
