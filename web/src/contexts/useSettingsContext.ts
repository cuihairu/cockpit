import { useContext } from 'react'
import { SettingsContext } from './settingsContextValue'

export const useSettingsContext = () => {
  const context = useContext(SettingsContext)
  if (!context) {
    throw new Error('useSettingsContext must be used within a SettingsProvider')
  }
  return context
}
