import { createContext, useContext } from 'react'

export type ThemeChoice = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'
/** Точка, из которой новая тема раскрывается кругом, — обычно центр нажатой кнопки. */
export type RevealOrigin = { x: number; y: number }
export type ThemeValue = {
  choice: ThemeChoice
  resolved: ResolvedTheme
  setChoice: (choice: ThemeChoice, origin?: RevealOrigin) => void
}

/** Ключ в localStorage; его же читает public/theme-init.js до отрисовки. */
export const themeStorageKey = 'diary-theme'

export const ThemeContext = createContext<ThemeValue | null>(null)

export function useTheme(): ThemeValue {
  const value = useContext(ThemeContext)
  if (!value) throw new Error('useTheme must be used within ThemeProvider')
  return value
}

export function readThemeChoice(): ThemeChoice {
  try {
    const stored = window.localStorage.getItem(themeStorageKey)
    return stored === 'light' || stored === 'dark' ? stored : 'system'
  } catch {
    return 'system'
  }
}
