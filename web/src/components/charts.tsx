import { useId, useMemo, useRef, useState, type ReactNode } from 'react'
import { Table2, BarChart3 } from 'lucide-react'
import { compact, formatNumber, formatPercent, niceTicks, type Datum, type LinePoint, type Tone } from '../lib/chartMath'

/**
 * Небольшая библиотека графиков без внешних зависимостей.
 * Правила: тонкие метки (столбец ≤ 24px, скругление 4px только у вершины), линия 2px,
 * сетка — волосяные линии, подписи цветом текста (не цветом серии), у каждого графика
 * есть подсказка при наведении и фокусе и табличный вид, чтобы значение не зависело от цвета.
 */

type TableSpec = { columns: string[]; rows: (string | number)[][] }

/** Карточка графика: заголовок, легенда, переключатель «график / таблица». */
export function ChartCard({ title, subtitle, legend, table, actions, children, className = '' }: {
  title: string
  subtitle?: string
  legend?: ReactNode
  table?: TableSpec
  actions?: ReactNode
  children: ReactNode
  className?: string
}): React.JSX.Element {
  const [asTable, setAsTable] = useState(false)
  const headingId = useId()
  return (
    <section className={`panel chart-card ${className}`} aria-labelledby={headingId}>
      <div className="chart-card-head">
        <div>
          <h2 id={headingId}>{title}</h2>
          {subtitle && <p className="muted">{subtitle}</p>}
        </div>
        <div className="chart-card-actions">
          {actions}
          {table && (
            <button
              type="button"
              className="icon-button chart-toggle"
              aria-pressed={asTable}
              aria-label={asTable ? 'Показать графиком' : 'Показать таблицей'}
              title={asTable ? 'Показать графиком' : 'Показать таблицей'}
              onClick={() => setAsTable(!asTable)}
            >
              {asTable ? <BarChart3 aria-hidden="true" /> : <Table2 aria-hidden="true" />}
            </button>
          )}
        </div>
      </div>
      {legend && !asTable && <div className="chart-legend">{legend}</div>}
      {asTable && table ? <DataTable {...table} caption={title} /> : children}
    </section>
  )
}

export function DataTable({ columns, rows, caption }: TableSpec & { caption?: string }): React.JSX.Element {
  return (
    <div className="responsive-table chart-table">
      <table>
        {caption && <caption className="sr-only">{caption}</caption>}
        <thead>
          <tr>{columns.map((column) => <th key={column} scope="col">{column}</th>)}</tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={index}>{row.map((cell, cellIndex) => <td key={cellIndex}>{cell}</td>)}</tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function LegendItem({ color, label, line = false }: { color: string; label: string; line?: boolean }): React.JSX.Element {
  return (
    <span className="legend-item">
      <span className={line ? 'legend-line' : 'legend-swatch'} style={{ background: color }} aria-hidden="true" />
      {label}
    </span>
  )
}

/** Вертикальные столбцы: значение на вершине, подпись под осью, подсказка при наведении и фокусе. */
export function ColumnChart({ data, max, height = 220, format = (value) => formatNumber(value), ariaLabel, emptyText = 'Нет данных' }: {
  data: Datum[]
  max?: number
  height?: number
  format?: (value: number) => string
  ariaLabel: string
  emptyText?: string
}): React.JSX.Element {
  const top = max ?? Math.max(...data.map((item) => item.value), 0)
  // При заданном максимуме (шкала оценок, доля) деления строятся по нему и не выходят за предел.
  const ticks = max !== undefined ? niceTicks(max, max <= 10 && Number.isInteger(max) ? max : 4).filter((tick) => tick <= max) : niceTicks(top || 1)
  const domain = max ?? ticks[ticks.length - 1]!
  const [active, setActive] = useState<string | null>(null)
  if (!data.length) return <p className="chart-empty">{emptyText}</p>
  return (
    <div className="column-chart" style={{ height }} role="group" aria-label={ariaLabel}>
      <div className="chart-grid" aria-hidden="true">
        {ticks.map((tick) => (
          <div key={tick} className="chart-gridline" style={{ bottom: `${(tick / domain) * 100}%` }}>
            <span>{compact(tick)}</span>
          </div>
        ))}
      </div>
      <div className="column-plot">
        {data.map((item) => {
          const share = domain > 0 ? Math.max(0, Math.min(1, item.value / domain)) : 0
          const label = `${item.label}: ${format(item.value)}${item.hint ? `, ${item.hint}` : ''}`
          return (
            <div key={item.key} className="column-slot">
              <button
                type="button"
                className={`column-hit ${active === item.key ? 'active' : ''}`}
                aria-label={label}
                onMouseEnter={() => setActive(item.key)}
                onMouseLeave={() => setActive(null)}
                onFocus={() => setActive(item.key)}
                onBlur={() => setActive(null)}
              >
                <span className="column-value">{format(item.value)}</span>
                <span className="column-bar" style={{ height: `${share * 100}%`, background: item.color ?? 'var(--series-1)' }} />
              </button>
              <span className="column-label" title={item.label}>{item.label}</span>
              {active === item.key && (
                <span className="chart-tooltip" role="tooltip">
                  <strong>{format(item.value)}</strong>
                  <span>{item.label}</span>
                  {item.hint && <small>{item.hint}</small>}
                </span>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

/** Горизонтальные полосы — когда подписи длинные (классы, предметы, маршруты). */
export function BarList({ data, max, format = (value) => formatNumber(value), ariaLabel }: {
  data: Datum[]
  max?: number
  format?: (value: number) => string
  ariaLabel: string
}): React.JSX.Element {
  const domain = max ?? Math.max(...data.map((item) => item.value), 0)
  if (!data.length) return <p className="chart-empty">Нет данных</p>
  return (
    <ul className="bar-list" aria-label={ariaLabel}>
      {data.map((item) => {
        const share = domain > 0 ? Math.max(0, Math.min(1, item.value / domain)) : 0
        return (
          <li key={item.key} title={item.hint}>
            <span className="bar-list-label">{item.label}</span>
            <span className="bar-list-track" aria-hidden="true">
              <span className="bar-list-fill" style={{ width: `${Math.max(share * 100, item.value > 0 ? 2 : 0)}%`, background: item.color ?? 'var(--series-1)' }} />
            </span>
            <span className="bar-list-value">{format(item.value)}</span>
          </li>
        )
      })}
    </ul>
  )
}

/** Сегментированная полоса «целое из частей» (≤ 6 частей) с легендой. Заменяет круговую диаграмму. */
export function SegmentedBar({ parts, ariaLabel, format = (value) => formatNumber(value, 0) }: {
  parts: Datum[]
  ariaLabel: string
  format?: (value: number) => string
}): React.JSX.Element {
  const total = parts.reduce((sum, part) => sum + Math.max(0, part.value), 0)
  const [active, setActive] = useState<string | null>(null)
  if (!total) return <p className="chart-empty">Нет данных</p>
  return (
    <div className="segmented-figure">
      <div className="segmented-bar" role="group" aria-label={ariaLabel}>
        {parts.filter((part) => part.value > 0).map((part) => {
          const share = part.value / total
          return (
            <button
              type="button"
              key={part.key}
              className={`segment ${active === part.key ? 'active' : ''}`}
              style={{ flexGrow: part.value, background: part.color }}
              aria-label={`${part.label}: ${format(part.value)} (${formatPercent(share)})`}
              onMouseEnter={() => setActive(part.key)}
              onMouseLeave={() => setActive(null)}
              onFocus={() => setActive(part.key)}
              onBlur={() => setActive(null)}
            >
              {share >= 0.12 && <span>{formatPercent(share)}</span>}
            </button>
          )
        })}
      </div>
      <div className="chart-legend">
        {parts.map((part) => (
          <span key={part.key} className={`legend-item ${active === part.key ? 'active' : ''}`}>
            <span className="legend-swatch" style={{ background: part.color }} aria-hidden="true" />
            {part.label} <strong>{format(part.value)}</strong>
          </span>
        ))}
      </div>
    </div>
  )
}

/** Индикатор доли (0…1). Цвет заполнения передаёт состояние, подпись с числом обязательна. */
export function Meter({ value, label, tone = 'info', detail }: { value: number | null | undefined; label: string; tone?: Tone; detail?: string }): React.JSX.Element {
  const share = value === null || value === undefined ? 0 : Math.max(0, Math.min(1, value))
  return (
    <div className="meter">
      <div className="meter-head">
        <span>{label}</span>
        <strong>{value === null || value === undefined ? '—' : formatPercent(share, share < 0.1 ? 1 : 0)}</strong>
      </div>
      <div
        className={`meter-track tone-${tone}`}
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(share * 100)}
      >
        <span style={{ width: `${share * 100}%` }} />
      </div>
      {detail && <small>{detail}</small>}
    </div>
  )
}


/**
 * Линейный график одной серии: линия 2px, заливка 10%, точка на конце, перекрестие с подсказкой.
 * Клавиатура: фокус на графике и стрелки ←/→ перемещают выбранную точку.
 */
export function LineChart({ points, domain, height = 200, format = (value) => formatNumber(value), ariaLabel, color = 'var(--series-1)' }: {
  points: LinePoint[]
  domain?: [number, number]
  height?: number
  format?: (value: number) => string
  ariaLabel: string
  color?: string
}): React.JSX.Element {
  const container = useRef<HTMLDivElement>(null)
  const [index, setIndex] = useState<number | null>(null)
  const [min, max] = useMemo<[number, number]>(() => {
    if (domain) return domain
    const values = points.map((point) => point.value)
    const top = niceTicks(Math.max(...values, 0) || 1)
    return [0, top[top.length - 1]!]
  }, [domain, points])
  if (points.length === 0) return <p className="chart-empty">Нет данных</p>
  const span = max - min || 1
  const x = (i: number) => (points.length === 1 ? 50 : (i / (points.length - 1)) * 100)
  const y = (value: number) => 100 - ((Math.max(min, Math.min(max, value)) - min) / span) * 100
  const line = points.map((point, i) => `${i === 0 ? 'M' : 'L'}${x(i)},${y(point.value)}`).join(' ')
  const area = `${line} L${x(points.length - 1)},100 L${x(0)},100 Z`
  const ticks = niceTicks(max - min || 1).map((tick) => tick + min).filter((tick) => tick <= max)
  const pick = (clientX: number) => {
    const box = container.current?.getBoundingClientRect()
    if (!box || box.width === 0) return
    const ratio = (clientX - box.left) / box.width
    setIndex(Math.max(0, Math.min(points.length - 1, Math.round(ratio * (points.length - 1)))))
  }
  const selected = index === null ? null : points[index]
  const last = points[points.length - 1]!
  return (
    <div className="line-chart" style={{ height }}>
      <div className="chart-grid" aria-hidden="true">
        {ticks.map((tick) => (
          <div key={tick} className="chart-gridline" style={{ bottom: `${((tick - min) / span) * 100}%` }}>
            <span>{compact(Number(tick.toFixed(2)))}</span>
          </div>
        ))}
      </div>
      <div
        ref={container}
        className="line-plot"
        role="img"
        tabIndex={0}
        aria-label={`${ariaLabel}. Последнее значение ${format(last.value)} (${last.label}). Стрелками можно просмотреть точки.`}
        onPointerMove={(event) => pick(event.clientX)}
        onPointerLeave={() => setIndex(null)}
        onFocus={() => setIndex(points.length - 1)}
        onBlur={() => setIndex(null)}
        onKeyDown={(event) => {
          if (event.key === 'ArrowLeft') setIndex((current) => Math.max(0, (current ?? points.length - 1) - 1))
          if (event.key === 'ArrowRight') setIndex((current) => Math.min(points.length - 1, (current ?? 0) + 1))
        }}
      >
        <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
          <path d={area} fill={color} opacity={0.1} />
          <path d={line} fill="none" stroke={color} strokeWidth={2} vectorEffect="non-scaling-stroke" strokeLinejoin="round" strokeLinecap="round" />
        </svg>
        <span className="line-dot" style={{ left: `${x(points.length - 1)}%`, top: `${y(last.value)}%`, background: color }} aria-hidden="true" />
        {selected && index !== null && (
          <>
            <span className="crosshair" style={{ left: `${x(index)}%` }} aria-hidden="true" />
            <span className="line-dot active" style={{ left: `${x(index)}%`, top: `${y(selected.value)}%`, background: color }} aria-hidden="true" />
            <span className={`chart-tooltip line-tooltip ${x(index) > 70 ? 'flip' : ''}`} style={{ left: `${x(index)}%` }} role="tooltip">
              <strong>{format(selected.value)}</strong>
              <span>{selected.label}</span>
              {selected.hint && <small>{selected.hint}</small>}
            </span>
          </>
        )}
      </div>
      <div className="line-axis" aria-hidden="true">
        <span>{points[0]!.label}</span>
        {points.length > 2 && <span>{points[Math.floor((points.length - 1) / 2)]!.label}</span>}
        <span>{last.label}</span>
      </div>
    </div>
  )
}

/** Маленький график тренда без осей — в плитке показателя. */
export function Sparkline({ values, color = 'var(--series-1)', label }: { values: number[]; color?: string; label: string }): React.JSX.Element | null {
  if (values.length < 2) return null
  const top = Math.max(...values)
  const bottom = Math.min(...values)
  const span = top - bottom || 1
  const path = values.map((value, i) => `${i === 0 ? 'M' : 'L'}${(i / (values.length - 1)) * 100},${100 - ((value - bottom) / span) * 100}`).join(' ')
  return (
    <svg className="sparkline" viewBox="0 0 100 100" preserveAspectRatio="none" role="img" aria-label={label}>
      <path d={path} fill="none" stroke={color} strokeWidth={2} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
    </svg>
  )
}

/** Плитка показателя: подпись, значение, пояснение, по желанию — тренд. */
export function StatTile({ icon, label, value, detail, tone = 'blue', trend }: {
  icon: ReactNode
  label: string
  value: ReactNode
  detail?: ReactNode
  tone?: 'blue' | 'green' | 'amber' | 'violet' | 'rose' | 'teal'
  trend?: ReactNode
}): React.JSX.Element {
  return (
    <article className={`stat-tile stat-${tone}`}>
      <span className="stat-icon" aria-hidden="true">{icon}</span>
      <div className="stat-body">
        <span className="stat-label">{label}</span>
        <strong className="stat-value">{value}</strong>
        {detail && <span className="stat-detail">{detail}</span>}
      </div>
      {trend && <div className="stat-trend">{trend}</div>}
    </article>
  )
}
