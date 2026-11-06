// Ставит тему до первой отрисовки, чтобы тёмная тема не мигала белым. Отдельный файл, а не встроенный скрипт:
// CSP разрешает только script-src 'self'. Ключ и значения совпадают с src/themeContext.ts.
(function () {
  var choice = 'system'
  try { choice = window.localStorage.getItem('diary-theme') || 'system' } catch (error) { /* хранилище недоступно */ }
  var dark = choice === 'dark' || (choice !== 'light' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
})()
