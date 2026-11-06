import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useMediaQuery } from './hooks/useMediaQuery'
import { ThemeContext, readThemeChoice, themeStorageKey, type ResolvedTheme, type RevealOrigin, type ThemeChoice } from './themeContext'

const themeColors: Record<ResolvedTheme, string> = { light: '#1e3b32', dark: '#12271f' }

function applyTheme(theme: ResolvedTheme): void {
  document.documentElement.dataset.theme = theme
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', themeColors[theme])
}

/**
 * Тема «светлая / тёмная / как в системе». Выбор хранится в браузере; при смене новая тема раскрывается кругом
 * от нажатой кнопки (View Transitions), если браузер это умеет и не включено «уменьшить движение».
 * Печать всегда в светлой теме — распечатка архива журнала не должна зависеть от вечернего режима.
 */
export function ThemeProvider({ children }: { children: ReactNode }): React.JSX.Element {
  const [choice, setChoiceState] = useState<ThemeChoice>(readThemeChoice)
  const systemDark = useMediaQuery('(prefers-color-scheme: dark)')
  const reduceMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
  const resolved: ResolvedTheme = choice === 'system' ? (systemDark ? 'dark' : 'light') : choice

  useEffect(() => { applyTheme(resolved) }, [resolved])

  useEffect(() => {
    const before = () => { document.documentElement.dataset.theme = 'light' }
    const after = () => { document.documentElement.dataset.theme = resolved }
    window.addEventListener('beforeprint', before)
    window.addEventListener('afterprint', after)
    return () => { window.removeEventListener('beforeprint', before); window.removeEventListener('afterprint', after) }
  }, [resolved])

  const setChoice = useCallback((next: ThemeChoice, origin?: RevealOrigin) => {
    try { window.localStorage.setItem(themeStorageKey, next) } catch { /* выбор действует до перезагрузки */ }
    const nextResolved: ResolvedTheme = next === 'system' ? (systemDark ? 'dark' : 'light') : next
    const root = document.documentElement
    if (nextResolved === root.dataset.theme || reduceMotion || typeof document.startViewTransition !== 'function') {
      setChoiceState(next)
      return
    }
    const x = origin?.x ?? window.innerWidth / 2
    const y = origin?.y ?? 0
    const radius = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y))
    root.style.setProperty('--reveal-x', `${x}px`)
    root.style.setProperty('--reveal-y', `${y}px`)
    root.style.setProperty('--reveal-r', `${radius}px`)
    root.classList.add('theme-switching')
    const transition = document.startViewTransition(() => applyTheme(nextResolved))
    void transition.finished.finally(() => root.classList.remove('theme-switching'))
    setChoiceState(next)
  }, [systemDark, reduceMotion])

  const value = useMemo(() => ({ choice, resolved, setChoice }), [choice, resolved, setChoice])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}
