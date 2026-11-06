import { flushSync } from 'react-dom'

/**
 * Выполняет обновление интерфейса внутри View Transition, чтобы смена страницы была плавной. React Router даёт
 * `viewTransition` только в data-режиме, а приложение использует BrowserRouter, поэтому переход оборачиваем сами.
 * Без поддержки браузера или при «уменьшить движение» обновление выполняется сразу.
 */
export function withViewTransition(update: () => void): void {
  const reduced = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (reduced || typeof document.startViewTransition !== 'function') {
    update()
    return
  }
  document.startViewTransition(() => flushSync(update))
}

/** Обычный щелчок без модификаторов: только его перехватываем, Ctrl/Shift/средняя кнопка открывают как обычно. */
export function isPlainClick(event: React.MouseEvent<HTMLElement>): boolean {
  return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey && !event.defaultPrevented
}
