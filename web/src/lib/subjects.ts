import type { CSSProperties } from 'react'

/** Полоса предмета и бледная заливка карточки. Цвет следует за предметом, а не за его позицией в списке. */
const palette: { stripe: string; wash: string }[] = [
  { stripe: '#2a78d6', wash: '#e8f1fd' },
  { stripe: '#eb6834', wash: '#fdeee7' },
  { stripe: '#1baf7a', wash: '#e3f6ef' },
  { stripe: '#eda100', wash: '#fdf3dc' },
  { stripe: '#e87ba4', wash: '#fdedf3' },
  { stripe: '#008300', wash: '#e2f2e2' },
  { stripe: '#6250d6', wash: '#eeecfb' },
  { stripe: '#e34948', wash: '#fdeaea' },
]

function hash(value: string): number {
  let result = 0
  for (const char of value.trim().toLowerCase()) result = (result * 31 + char.codePointAt(0)!) >>> 0
  return result
}

export function subjectColor(name?: string | null): { stripe: string; wash: string } {
  if (!name) return { stripe: 'var(--accent)', wash: 'var(--surface-2)' }
  return palette[hash(name) % palette.length]!
}

/** CSS-переменные --subject и --subject-wash для карточек урока и строк расписания. */
export function subjectStyle(name?: string | null): CSSProperties {
  const color = subjectColor(name)
  return { '--subject': color.stripe, '--subject-wash': color.wash } as CSSProperties
}
