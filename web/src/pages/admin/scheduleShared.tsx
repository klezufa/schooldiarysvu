import { useEffect, useState } from 'react'
import { CopyPlus } from 'lucide-react'
import { mutation } from '../../api/client'
import type { AcademicPeriod, CarryOverResult, NamedEntity } from '../../api/types'
import { useToast } from '../../components/toastContext'

/**
 * Назначения учителей и шаблон расписания действуют в пределах учебного периода. В новой четверти их нет, и выбрать
 * предмет в шаблоне нельзя — предлагаем перенести всё из предыдущего периода одной кнопкой (повторы сервер пропускает).
 * Если предыдущий период — из прошлого учебного года, классы там другие (5А прошлого года → 6А нового): администратор
 * выбирает, из какого класса переносить.
 */
export function CarryOverNotice({ period, previous, classId, className, hasAssignments, sourceClasses, onDone, plain = false }: {
  period: AcademicPeriod; previous: AcademicPeriod; classId: string; className: string; hasAssignments: boolean
  sourceClasses: NamedEntity[]; onDone: () => Promise<void>
  /** Упрощённый режим: без «назначений» и «шаблона» — администратор видит только расписание. */
  plain?: boolean
}): React.JSX.Element {
  const { showToast } = useToast()
  const crossYear = previous.academic_year_id !== period.academic_year_id
  const [allClasses, setAllClasses] = useState(false)
  const [sourceClass, setSourceClass] = useState('')
  const [pending, setPending] = useState(false)
  useEffect(() => {
    if (crossYear && !sourceClasses.some((item) => item.id === sourceClass)) {
      setSourceClass((sourceClasses.find((item) => item.name === className) ?? sourceClasses[0])?.id ?? '')
    }
  }, [crossYear, sourceClasses, sourceClass, className])
  const missing = !hasAssignments ? 'нет назначений учителей — поэтому в шаблоне нельзя выбрать предмет' : 'пустой шаблон расписания'
  const carryOver = async () => {
    setPending(true)
    try {
      const body = crossYear
        ? { from_period_id: previous.id, class_id: sourceClass, target_class_id: classId }
        : { from_period_id: previous.id, class_id: allClasses ? undefined : classId }
      const result = await mutation<CarryOverResult>(`/academic-periods/${period.id}/carry-over`, 'POST', body)
      const skipped = result.assignments_skipped + result.entries_skipped
      showToast(`Перенесено назначений: ${result.assignments_created}, уроков шаблона: ${result.entries_created}`
        + (skipped ? `. Пропущено ${skipped}: уже есть, слот занят или учитель неактивен` : ''))
      await onDone()
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось перенести', 'error')
    } finally {
      setPending(false)
    }
  }
  return (
    <div className="notice info period-hint carry-over" role="status">
      <CopyPlus aria-hidden="true" />
      {plain ? (
        <span>
          <strong>У {className} ещё нет расписания на «{period.name}».</strong> Можно перенести его из «{previous.name}» вместе с учителями
          {crossYear ? <> — это новый учебный год, выберите прошлогодний класс</> : null} — или составить заново, нажимая на клетки.
        </span>
      ) : <span>
        <strong>В периоде «{period.name}» у класса {className} {missing}.</strong> Назначения и шаблон действуют в пределах периода —
        {crossYear
          ? <> это новый учебный год: выберите класс прошлого года, чьи учителя и шаблон достанутся {className}.</>
          : <> перенесите их из «{previous.name}».</>}
        {' '}Уже существующие назначения и занятые слоты не изменятся.
      </span>}
      {crossYear ? (
        <label className="checkbox-label">Из класса
          <select aria-label="Класс прошлого года" value={sourceClass} onChange={(event) => setSourceClass(event.target.value)}>
            {sourceClasses.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </label>
      ) : (
        <label className="checkbox-label"><input type="checkbox" checked={allClasses} onChange={(event) => setAllClasses(event.target.checked)} />Для всех классов</label>
      )}
      <button type="button" className="button small primary" disabled={pending || (crossYear && !sourceClass)} onClick={carryOver}>{pending ? 'Переносим…' : `Перенести из «${previous.name}»`}</button>
    </div>
  )
}
