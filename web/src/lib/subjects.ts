import type { CSSProperties } from 'react'

/** Полоса предмета и бледная заливка карточки. Цвет следует за предметом, а не за его позицией в списке;
 *  сами оттенки — токены --subj-N в styles/tokens.css, у тёмной темы свои ступени. */
const palette: { stripe: string; wash: string }[] = Array.from({ length: 8 }, (_, index) => ({
  stripe: `var(--subj-${index + 1})`,
  wash: `var(--subj-${index + 1}-wash)`,
}))

/** FNV-1a: у хеша «× 31» остаток от деления на 8 почти не зависит от букв, и разные предметы получали один цвет. */
function hash(value: string): number {
  let result = 0x811c9dc5
  for (const char of value.trim().toLowerCase()) {
    result ^= char.codePointAt(0)!
    result = Math.imul(result, 0x01000193)
  }
  return result >>> 16
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
