import { createContext } from 'react'
import type { SettingsContextType } from './settingsTypes'

export const SettingsContext = createContext<SettingsContextType | undefined>(undefined)
