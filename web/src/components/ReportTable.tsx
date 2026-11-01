import type { ReportRecord } from '../api/types'
import { valueText } from '../lib/format'

const columnNames: Record<string, string> = {
  student_id: 'ID ученика', display_name: 'Ученик', subject_name: 'Предмет', grade_count: 'Количество оценок', average: 'Средний балл', status: 'Статус', count: 'Количество', lesson_date: 'Дата', class_name: 'Класс', work_count: 'Работ', attendance_count: 'Отметок посещаемости', id: 'ID', student_name: 'Ученик', value: 'Значение',
}

export function ReportTable({ data }: { data: ReportRecord[] }): React.JSX.Element {
  if (!data.length) return <div className="state-view"><h3>Нет данных для отчёта</h3><p>Измените фильтры или учебный период.</p></div>
  const columns = [...new Set(data.flatMap((row) => Object.keys(row)))].filter((key) => !key.endsWith('_id') || key === 'student_id')
  return <div className="responsive-table"><table><thead><tr>{columns.map((column) => <th key={column}>{columnNames[column] ?? column}</th>)}</tr></thead><tbody>{data.map((row, index) => <tr key={index}>{columns.map((column) => <td key={column}>{valueText(row[column])}</td>)}</tr>)}</tbody></table></div>
}
