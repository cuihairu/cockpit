import { useEffect, useState, type ReactNode } from 'react'
import { SettingsContext } from './settingsContextValue'
import type { UISettings } from './settingsTypes'

const SETTINGS_STORAGE_KEY = 'cockpit.ui.settings'

const DEFAULT_SETTINGS: UISettings = {
  siteName: 'Cockpit',
  refreshInterval: 30,
  enableNotifications: true,
  theme: 'light',
  compactMode: false,
  showResourceCount: true,
}

const normalizeSettings = (input: Partial<UISettings> | null | undefined): UISettings => {
  const refreshInterval = Number(input?.refreshInterval)

  return {
    siteName: input?.siteName?.trim() || DEFAULT_SETTINGS.siteName,
    refreshInterval: Number.isFinite(refreshInterval) && refreshInterval >= 5 && refreshInterval <= 300
      ? refreshInterval
      : DEFAULT_SETTINGS.refreshInterval,
    enableNotifications: input?.enableNotifications ?? DEFAULT_SETTINGS.enableNotifications,
    theme: input?.theme === 'dark' || input?.theme === 'auto' ? input.theme : DEFAULT_SETTINGS.theme,
    compactMode: input?.compactMode ?? DEFAULT_SETTINGS.compactMode,
    showResourceCount: input?.showResourceCount ?? DEFAULT_SETTINGS.showResourceCount,
  }
}

const loadStoredSettings = (): UISettings => {
  const raw = localStorage.getItem(SETTINGS_STORAGE_KEY)
  if (!raw) {
    return DEFAULT_SETTINGS
  }

  try {
    return normalizeSettings(JSON.parse(raw) as Partial<UISettings>)
  } catch {
    return DEFAULT_SETTINGS
  }
}

export const SettingsProvider = ({ children }: { children: ReactNode }) => {
  const [settings, setSettings] = useState<UISettings>(() => loadStoredSettings())

  useEffect(() => {
    localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(settings))
  }, [settings])

  const updateSettings = (patch: Partial<UISettings>) => {
    setSettings((current) => normalizeSettings({ ...current, ...patch }))
  }

  return (
    <SettingsContext.Provider value={{ settings, updateSettings }}>
      {children}
    </SettingsContext.Provider>
  )
}
