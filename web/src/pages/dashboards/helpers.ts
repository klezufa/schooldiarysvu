import { gradeTone, toneColor, type Datum } from '../../lib/chartMath'
import { addDays, dateISO, formatDate } from '../../lib/dates'
import type { SchoolAnalytics, StudentAnalytics } from '../../api/types'

/** «Иванов Иван Петрович» → «Иван»; одно слово возвращается как есть. */
export function firstName(displayName?: string): string {
  const parts = (displayName ?? '').trim().split(/\s+/)
  return parts.length >= 2 ? parts[1]! : parts[0] ?? ''
}

export function greeting(now = new Date()): string {
  const hour = now.getHours()
  if (hour < 6) return 'Доброй ночи'
  if (hour < 12) return 'Доброе утро'
  if (hour < 18) return 'Добрый день'
  return 'Добрый вечер'
}

/** Подпись срока: «Сегодня», «Завтра» или дата. */
export function dueLabel(date: string): { text: string; tone: 'danger' | 'warning' | 'info' | 'neutral' } {
  const today = dateISO()
  if (date < today) return { text: 'Просрочено', tone: 'danger' }
  if (date === today) return { text: 'Сегодня', tone: 'warning' }
  if (date === addDays(today, 1)) return { text: 'Завтра', tone: 'info' }
  return { text: formatDate(date, { day: 'numeric', month: 'short' }), tone: 'neutral' }
}

export function gradeColumns(subjects: StudentAnalytics['subjects'], max = 5): Datum[] {
  return subjects
    .filter((subject) => subject.average !== null && subject.average !== undefined)
    .map((subject) => ({
      key: subject.subject_id,
      label: subject.subject_name,
      value: subject.average!,
      color: toneColor(gradeTone(subject.average!, max)),
      hint: `${subject.grades_count} оценок${subject.term_value != null ? `, итоговая ${subject.term_value}` : ''}`,
    }))
}

const gradeLabels: Record<number, string> = { 5: 'Отлично', 4: 'Хорошо', 3: 'Удовлетворительно', 2: 'Неудовлетворительно', 1: 'Плохо' }

/** Распределение оценок → части сегментированной полосы, от высших к низшим. */
export function distributionParts(distribution: SchoolAnalytics['distribution'], max = 5): Datum[] {
  return [...distribution]
    .sort((a, b) => b.value - a.value)
    .map((item) => ({
      key: String(item.value),
      label: max === 5 && gradeLabels[item.value] ? `«${item.value}» ${gradeLabels[item.value]!.toLowerCase()}` : `«${item.value}»`,
      value: item.count,
      color: toneColor(gradeTone(item.value, max)),
    }))
}

export function weekLabel(weekStart: string): string {
  return formatDate(weekStart, { day: 'numeric', month: 'short' })
}

export function averageTone(value?: number | null, max = 5): 'green' | 'blue' | 'amber' | 'rose' {
  if (value === null || value === undefined) return 'blue'
  const tone = gradeTone(value, max)
  return tone === 'good' ? 'green' : tone === 'info' ? 'blue' : tone === 'warning' ? 'amber' : 'rose'
}

/** Последний завершившийся период — ему предлагают переключиться, если текущий ещё пуст. */
export function previousPeriod<T extends { starts_on: string; ends_on: string }>(periods: T[] | undefined, today: string): T | undefined {
  return [...(periods ?? [])].filter((period) => period.ends_on < today).sort((a, b) => b.ends_on.localeCompare(a.ends_on))[0]
}
