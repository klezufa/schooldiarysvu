/** Числа, форматы и цвета для графиков — без React, чтобы их импортировали и компоненты, и тесты. */

export type Datum = { key: string; label: string; value: number; color?: string; hint?: string }
export type LinePoint = { key: string; label: string; value: number; hint?: string }
export type Tone = 'good' | 'info' | 'warning' | 'critical' | 'neutral'

const numberFormat = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 2 })
const compactFormat = new Intl.NumberFormat('ru-RU', { notation: 'compact', maximumFractionDigits: 1 })

export function formatNumber(value: number, digits = 2): string {
  return new Intl.NumberFormat('ru-RU', { maximumFractionDigits: digits }).format(value)
}

export function formatPercent(value: number | null | undefined, digits = 0): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return new Intl.NumberFormat('ru-RU', { style: 'percent', maximumFractionDigits: digits }).format(value)
}

export function formatBytes(value: number | null | undefined): string {
  if (value === null || value === undefined) return '—'
  const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ']
  let size = value
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${formatNumber(size, size >= 100 ? 0 : 1)} ${units[unit]}`
}

/** Подписи оси: целиком до 10 000, дальше — компактно (12,5 тыс.). */
export function compact(value: number): string {
  return Math.abs(value) >= 10_000 ? compactFormat.format(value) : numberFormat.format(value)
}

/** Тон оценки по доле от максимума шкалы: 5 — хорошо, 4 — нормально, 3 — внимание, ниже — проблема. */
export function gradeTone(value: number, max = 5): Tone {
  const share = max > 0 ? value / max : 0
  if (share >= 0.9) return 'good'
  if (share >= 0.7) return 'info'
  if (share >= 0.5) return 'warning'
  return 'critical'
}

export function toneColor(tone: Tone): string {
  return `var(--tone-${tone})`
}

/** «Красивые» деления оси: 0…max с шагом 1/2/2,5/5 × 10ⁿ, последнее деление не меньше max. */
export function niceTicks(max: number, count = 4): number[] {
  if (!(max > 0)) return [0, 1]
  const rough = max / count
  const magnitude = 10 ** Math.floor(Math.log10(rough))
  const step = [1, 2, 2.5, 5, 10].map((factor) => factor * magnitude).find((candidate) => candidate >= rough) ?? rough
  const ticks: number[] = []
  for (let tick = 0; tick <= max + step * 0.001; tick += step) ticks.push(Number(tick.toFixed(6)))
  if (ticks[ticks.length - 1]! < max) ticks.push(Number((ticks[ticks.length - 1]! + step).toFixed(6)))
  return ticks
}

/** Цвета серий в фиксированном порядке, проверенном на различимость при дальтонизме. Цвет следует за сущностью, не за рангом. */
export const seriesColors = ['var(--series-1)', 'var(--series-2)', 'var(--series-3)', 'var(--series-4)', 'var(--series-5)', 'var(--series-6)', 'var(--series-7)', 'var(--series-8)']
