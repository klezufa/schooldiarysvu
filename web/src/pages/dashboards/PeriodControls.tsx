import { CalendarRange, Info } from 'lucide-react'
import type { AcademicPeriod } from '../../api/types'

/** Выбор учебного периода дашборда. Пустое значение — «текущий», его определяет сервер. */
export function PeriodSelect({ value, onChange, periods }: { value: string; onChange: (id: string) => void; periods?: AcademicPeriod[] }): React.JSX.Element | null {
  if (!periods?.length) return null
  return (
    <label className="period-select">
      <CalendarRange aria-hidden="true" />
      <span className="sr-only">Учебный период</span>
      <select value={value} onChange={(event) => onChange(event.target.value)} aria-label="Учебный период">
        <option value="">Текущий период</option>
        {periods.map((period) => <option key={period.id} value={period.id}>{period.name}</option>)}
      </select>
    </label>
  )
}

/** Подсказка, когда в только что начавшемся периоде ещё нет оценок. */
export function EmptyPeriodHint({ previous, onShow }: { previous?: AcademicPeriod; onShow: (id: string) => void }): React.JSX.Element | null {
  if (!previous) return null
  return (
    <div className="notice info period-hint" role="status">
      <Info aria-hidden="true" />
      <span>В текущем периоде оценок пока нет — графики заполнятся по мере работы.</span>
      <button type="button" className="button small secondary" onClick={() => onShow(previous.id)}>Показать: {previous.name}</button>
    </div>
  )
}
