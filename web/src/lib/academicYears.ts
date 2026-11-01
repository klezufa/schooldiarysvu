import type { AcademicPeriod, AcademicYear, NamedEntity } from '../api/types'
import { dateISO } from './dates'

/**
 * Классы относятся к учебному году: после перехода на новый год «5А» прошлого года и «5А» нового — разные классы
 * с одинаковым названием. Эти функции не дают их перепутать: подписывают год и отбирают классы нужного года.
 */

/** Подпись класса: год добавляется, только если в списке есть классы разных лет. */
export function classLabeler(classes: NamedEntity[] | undefined, years: AcademicYear[] | undefined): (item: NamedEntity) => string {
  const multiYear = new Set((classes ?? []).map((item) => item.academic_year_id)).size > 1
  return (item) => {
    if (!multiYear) return item.name
    const year = years?.find((candidate) => candidate.id === item.academic_year_id)
    return year ? `${item.name} · ${year.name}` : item.name
  }
}

/** Классы одного учебного года. */
export function classesOfYear(classes: NamedEntity[] | undefined, yearId: string | undefined): NamedEntity[] {
  return (classes ?? []).filter((item) => item.academic_year_id === yearId)
}

/** Классы незавершённых лет (текущий и подготовленный следующий): в них можно зачислять и приглашать. */
export function openClasses(classes: NamedEntity[] | undefined, years: AcademicYear[] | undefined, today = dateISO()): NamedEntity[] {
  const open = new Set((years ?? []).filter((year) => year.ends_on >= today).map((year) => year.id))
  return (classes ?? []).filter((item) => item.academic_year_id !== undefined && open.has(item.academic_year_id))
}

/**
 * Текущий период: тот, что идёт сегодня; в каникулы между периодами — последний начавшийся (а не первый в году);
 * до начала года — первый.
 */
export function pickCurrentPeriod(periods: AcademicPeriod[] | undefined, today = dateISO()): AcademicPeriod | undefined {
  if (!periods?.length) return undefined
  const sorted = [...periods].sort((a, b) => a.starts_on.localeCompare(b.starts_on))
  return sorted.find((period) => period.starts_on <= today && period.ends_on >= today)
    ?? sorted.filter((period) => period.starts_on <= today).at(-1)
    ?? sorted[0]
}

/** Предыдущий период (возможно, прошлого учебного года) — источник для переноса назначений и шаблона. */
export function previousPeriod(periods: AcademicPeriod[] | undefined, period: AcademicPeriod | undefined): AcademicPeriod | undefined {
  if (!period) return undefined
  return [...(periods ?? [])].filter((item) => item.ends_on < period.starts_on).sort((a, b) => a.starts_on.localeCompare(b.starts_on)).at(-1)
}

/** Дата по умолчанию внутри периода: сегодня, если период уже идёт, иначе его первый день. */
export function defaultDateIn(period: { starts_on: string; ends_on: string } | undefined, today = dateISO()): string {
  if (!period) return today
  if (today < period.starts_on || today > period.ends_on) return period.starts_on
  return today
}
