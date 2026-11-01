import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { api, download, queryString, saveBlob } from '../api/client'
import type { ReportRecord } from '../api/types'
import { useAcademic } from '../hooks/useAcademic'
import { PageHeader, QueryState } from '../components/UI'
import { ReportTable } from '../components/ReportTable'
import { useToast } from '../components/toastContext'

type ReportKind = 'class' | 'attendance' | 'unfilled' | 'subject'

export function ReportsPage({ classMode = false }: { classMode?: boolean }): React.JSX.Element {
  const { classes, subjects, periods, periodId, setPeriodId } = useAcademic()
  const [classId, setClassId] = useState('')
  const [subjectId, setSubjectId] = useState('')
  const [kind, setKind] = useState<ReportKind>(classMode ? 'attendance' : 'class')
  const { showToast } = useToast()
  useEffect(() => { if (!classId && classes.data?.length) setClassId(classes.data[0]!.id) }, [classId, classes.data])
  useEffect(() => { if (!subjectId && subjects.data?.length) setSubjectId(subjects.data[0]!.id) }, [subjectId, subjects.data])
  const path = kind === 'class' ? `/reports/classes/${classId}${queryString({ academic_period_id: periodId })}` : kind === 'attendance' ? `/reports/classes/${classId}/attendance${queryString({ academic_period_id: periodId })}` : kind === 'subject' ? `/reports/subjects/${subjectId}${queryString({ class_id: classId, academic_period_id: periodId })}` : `/reports/unfilled${queryString({ academic_period_id: periodId })}`
  const enabled = Boolean(periodId && (kind === 'unfilled' || classId) && (kind !== 'subject' || subjectId))
  const report = useQuery({ queryKey: ['report', kind, classId, subjectId, periodId], queryFn: () => api<ReportRecord[]>(path), enabled })
  const exportCsv = async () => {
    try {
      const result = await download(`/reports/classes/${classId}${queryString({ academic_period_id: periodId, format: 'csv' })}`)
      saveBlob(result.blob, result.filename)
      showToast('CSV-отчёт сформирован')
    } catch (error) { showToast(error instanceof Error ? error.message : 'Не удалось скачать отчёт', 'error') }
  }
  return <div><PageHeader eyebrow={classMode ? 'Классное руководство' : 'Аналитика школы'} title={classMode ? 'Мой класс' : 'Отчёты'} description="Сводки строятся по данным PostgreSQL за выбранный учебный период." actions={kind === 'class' ? <button className="button secondary" disabled={!classId || !periodId} onClick={exportCsv}><Download />Скачать CSV</button> : undefined} />
    <div className="toolbar report-filters">{!classMode && <div className="segmented" aria-label="Вид отчёта"><button className={kind === 'class' ? 'active' : ''} onClick={() => setKind('class')}>Успеваемость класса</button><button className={kind === 'attendance' ? 'active' : ''} onClick={() => setKind('attendance')}>Посещаемость</button><button className={kind === 'subject' ? 'active' : ''} onClick={() => setKind('subject')}>По предмету</button><button className={kind === 'unfilled' ? 'active' : ''} onClick={() => setKind('unfilled')}>Незаполненные уроки</button></div>}{kind !== 'unfilled' && <label>Класс<select value={classId} onChange={(event) => setClassId(event.target.value)}>{classes.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>}{kind === 'subject' && <label>Предмет<select value={subjectId} onChange={(event) => setSubjectId(event.target.value)}>{subjects.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>}<label>Учебный период<select value={periodId} onChange={(event) => setPeriodId(event.target.value)}>{periods.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label></div>
    <section className="panel report-panel"><QueryState isLoading={report.isLoading && enabled} error={report.error} retry={report.refetch} isEmpty={!enabled} emptyTitle="Выберите параметры отчёта"><ReportTable data={report.data ?? []} /></QueryState></section>
  </div>
}
