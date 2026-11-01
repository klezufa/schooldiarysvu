import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, queryString } from '../api/client'
import type { StudentAnalytics, StudentGrade } from '../api/types'
import { attendanceNames, workNames, type AttendanceStatus } from '../api/types'
import { useStudent } from '../studentContext'
import { useAcademic } from '../hooks/useAcademic'
import { formatDate } from '../lib/dates'
import { scopedKey } from '../lib/student'
import { PageHeader, QueryState, StatusBadge } from '../components/UI'
import { ChartCard, ColumnChart, LineChart, Meter } from '../components/charts'
import { formatNumber, formatPercent, gradeTone } from '../lib/chartMath'
import { gradeColumns, weekLabel } from './dashboards/helpers'

type AttendanceRow = { subject_id: string; subject_name: string; status: AttendanceStatus; count: number }
type StudentReport = { attendance?: AttendanceRow[] }

export function GradesPage(): React.JSX.Element {
  const { studentId } = useStudent()
  const { periodId, periods, setPeriodId } = useAcademic()
  const grades = useQuery({ queryKey: scopedKey('grades', studentId, periodId), queryFn: () => api<StudentGrade[]>(`/grades${queryString({ student_id: studentId, academic_period_id: periodId })}`), enabled: Boolean(studentId) })
  const report = useQuery({ queryKey: scopedKey('student-report', studentId, periodId), queryFn: () => api<StudentReport>(`/reports/students/${studentId}${queryString({ academic_period_id: periodId })}`), enabled: Boolean(studentId && periodId) })
  const analytics = useQuery({
    queryKey: scopedKey('analytics-student', studentId, periodId),
    queryFn: () => api<StudentAnalytics>(`/analytics/student${queryString({ student_id: studentId, academic_period_id: periodId })}`),
    enabled: Boolean(studentId && periodId),
  })
  const groups = useMemo(() => {
    const map = new Map<string, StudentGrade[]>()
    grades.data?.forEach((grade) => map.set(grade.subject_name, [...map.get(grade.subject_name) ?? [], grade]))
    return [...map.entries()]
  }, [grades.data])
  return <div><PageHeader eyebrow="Успеваемость" title="Оценки" description="Оценки по предметам и личная динамика без рейтингов и сравнения с одноклассниками." actions={<label className="inline-label">Период<select value={periodId} onChange={(event) => setPeriodId(event.target.value)}>{periods.data?.map((period) => <option key={period.id} value={period.id}>{period.name}</option>)}</select></label>} />
    {analytics.data && analytics.data.grades_count > 0 && (
      <div className="dash-grid grades-charts">
        <div className="col-7">
          <ChartCard title="Средний балл по предметам" subtitle={`Средневзвешенный, общий: ${analytics.data.average != null ? formatNumber(analytics.data.average) : '—'}`}
            table={{ columns: ['Предмет', 'Средний балл', 'Оценок'], rows: analytics.data.subjects.map((item) => [item.subject_name, item.average != null ? formatNumber(item.average) : '—', item.grades_count]) }}>
            <ColumnChart data={gradeColumns(analytics.data.subjects)} max={5} ariaLabel="Средний балл по предметам" />
          </ChartCard>
        </div>
        <div className="col-5">
          <ChartCard title="Динамика по неделям" subtitle={`Посещаемость: ${formatPercent(analytics.data.attendance_rate)}`}
            table={{ columns: ['Неделя', 'Средний балл'], rows: analytics.data.dynamics.map((item) => [weekLabel(item.week_start), formatNumber(item.average)]) }}>
            <LineChart points={analytics.data.dynamics.map((item) => ({ key: item.week_start, label: weekLabel(item.week_start), value: item.average, hint: `${item.grades_count} оценок` }))} domain={[1, 5]} ariaLabel="Средний балл по неделям" height={170} />
            <Meter label="Посещаемость" value={analytics.data.attendance_rate} tone={analytics.data.attendance_rate != null && analytics.data.attendance_rate < 0.85 ? 'warning' : 'good'} detail={`пропусков: ${analytics.data.absences}, опозданий: ${analytics.data.late_count}`} />
          </ChartCard>
        </div>
      </div>
    )}
    <QueryState isLoading={grades.isLoading} error={grades.error} retry={grades.refetch} isEmpty={!groups.length} emptyTitle="Оценок за период нет" emptyText="Новые оценки появятся после сохранения учителем."><div className="grade-subjects">{groups.map(([subject, items]) => {
      const totals = items.reduce((result, grade) => ({ sum: result.sum + grade.value * grade.weight, weight: result.weight + grade.weight }), { sum: 0, weight: 0 })
      const average = totals.weight ? totals.sum / totals.weight : 0
      return <section className="panel grade-subject" key={subject}><div className="section-heading"><div><h2>{subject}</h2><p>{items.length} оценок</p></div><div className="subject-average"><strong>{average.toFixed(2)}</strong><span>средневзвешенный балл</span></div></div><div className="responsive-table"><table><thead><tr><th>Дата</th><th>Работа</th><th>Оценка</th><th>Вес</th><th>Комментарий</th></tr></thead><tbody>{items.slice().reverse().map((grade) => <tr key={grade.id}><td>{formatDate(grade.lesson_date)}</td><td>{workNames[grade.work_type] ?? grade.work_type}</td><td><span className={`grade-chip tone-${gradeTone(grade.value)}`}>{grade.value}</span></td><td>{grade.weight}</td><td>{grade.comment || '—'}</td></tr>)}</tbody></table></div><details className="calculation-note"><summary>Как рассчитан средний балл</summary><p>Сумма произведений каждой оценки на вес работы делится на сумму весов: {totals.sum.toFixed(2)} ÷ {totals.weight.toFixed(2)} = {average.toFixed(2)}. Это не итоговая отметка за период.</p></details></section>
    })}</div></QueryState>
    <section className="panel attendance-summary"><div className="section-heading"><div><p className="eyebrow">Посещаемость</p><h2>Отметки по предметам</h2></div></div><QueryState isLoading={report.isLoading && Boolean(periodId)} error={report.error} retry={report.refetch} isEmpty={!report.data?.attendance?.length} emptyTitle="Отметок посещаемости нет"><div className="attendance-summary-grid">{report.data?.attendance?.map((row) => <div key={`${row.subject_id}-${row.status}`}><span>{row.subject_name}</span><StatusBadge tone={row.status === 'present' ? 'success' : row.status === 'absent' ? 'danger' : 'warning'}>{attendanceNames[row.status]}</StatusBadge><strong>{row.count}</strong></div>)}</div></QueryState></section>
  </div>
}
