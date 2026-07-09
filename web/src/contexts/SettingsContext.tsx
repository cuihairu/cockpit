import { useEffect, useState, type ReactNode } from 'react'
import { SettingsContext } from './settingsContextValue'
import type { UISettings, UITheme } from './settingsTypes'

const SETTINGS_STORAGE_KEYS = ['cockpit.ui.settings', 'cockpit:settings']

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

const resolveTheme = (theme: UITheme): Exclude<UITheme, 'auto'> => {
  if (theme !== 'auto') return theme
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

const loadStoredSettings = (): UISettings => {
  for (const key of SETTINGS_STORAGE_KEYS) {
    const raw = localStorage.getItem(key)
    if (!raw) {
      continue
    }

    try {
      return normalizeSettings(JSON.parse(raw) as Partial<UISettings>)
    } catch {
      continue
    }
  }

  return DEFAULT_SETTINGS
}

export const SettingsProvider = ({ children }: { children: ReactNode }) => {
  const [settings, setSettings] = useState<UISettings>(() => loadStoredSettings())
  const [resolvedTheme, setResolvedTheme] = useState<Exclude<UITheme, 'auto'>>(() => resolveTheme(settings.theme))

  useEffect(() => {
    const serialized = JSON.stringify(settings)
    for (const key of SETTINGS_STORAGE_KEYS) {
      localStorage.setItem(key, serialized)
    }
    window.dispatchEvent(new CustomEvent<UISettings>('cockpit:settings-changed', { detail: settings }))
  }, [settings])

  useEffect(() => {
    const updateResolvedTheme = () => setResolvedTheme(resolveTheme(settings.theme))
    updateResolvedTheme()

    if (typeof window.matchMedia !== 'function') return undefined

    const media = window.matchMedia('(prefers-color-scheme: dark)')
    media.addEventListener('change', updateResolvedTheme)
    return () => media.removeEventListener('change', updateResolvedTheme)
  }, [settings.theme])

  const updateSettings = (patch: Partial<UISettings>) => {
    setSettings((current) => normalizeSettings({ ...current, ...patch }))
  }

  return (
    <SettingsContext.Provider value={{ settings, updateSettings, resolvedTheme }}>
      {children}
    </SettingsContext.Provider>
  )
}
