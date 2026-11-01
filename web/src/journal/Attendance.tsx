import { useEffect, useRef } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCheck } from 'lucide-react'
import { api, mutation } from '../api/client'
import type { Attendance, AttendanceStatus, Student } from '../api/types'
import { attendanceNames } from '../api/types'
import { useToast } from '../components/toastContext'
import { QueryState } from '../components/UI'

const statuses = Object.keys(attendanceNames) as AttendanceStatus[]

export function AttendancePanel({ lessonId, students, compact = false }: { lessonId: string; students: Student[]; compact?: boolean }): React.JSX.Element {
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const key = ['attendance', lessonId]
  const idempotencyKey = useRef(crypto.randomUUID())
  useEffect(() => { idempotencyKey.current = crypto.randomUUID() }, [lessonId])
  const attendance = useQuery({ queryKey: key, queryFn: () => api<Attendance[]>(`/attendance?lesson_id=${lessonId}`) })
  const update = useMutation({
    mutationFn: ({ studentId, status, reason, comment }: { studentId: string; status: AttendanceStatus; reason?: string; comment?: string }) => mutation<Attendance>('/attendance', 'PUT', { lesson_id: lessonId, student_id: studentId, status, reason, comment }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
    onError: (error) => showToast(error.message, 'error'),
  })
  const allPresent = useMutation({
    mutationFn: () => mutation<{ created_or_updated: number; items: Attendance[] }>('/attendance/bulk', 'POST', { lesson_id: lessonId }, { 'Idempotency-Key': idempotencyKey.current }),
    // Сервер повторяет сохранённый ответ для того же ключа: после успеха нужен новый ключ,
    // иначе следующее «Все присутствуют» вернёт устаревший результат, ничего не применив.
    onSuccess: (result) => { idempotencyKey.current = crypto.randomUUID(); queryClient.setQueryData(key, result.items); showToast(`Посещаемость сохранена: ${result.created_or_updated} записей`) },
    onError: (error) => showToast(`${error.message} Повторная отправка использует тот же ключ идемпотентности.`, 'error'),
  })
  return <section className={`attendance-panel ${compact ? 'compact' : ''}`} aria-label="Посещаемость"><div className="section-heading"><div><h3>Посещаемость</h3><p>{attendance.data?.length ?? 0} из {students.length} отмечено</p></div><button className="button secondary" disabled={allPresent.isPending || !students.length} onClick={() => allPresent.mutate()}><CheckCheck />{allPresent.isPending ? 'Сохраняем…' : 'Все присутствуют'}</button></div>
    <QueryState isLoading={attendance.isLoading} error={attendance.error} retry={attendance.refetch} isEmpty={!students.length} emptyTitle="В классе нет учеников"><div className="attendance-list">{students.map((student) => {
      const record = attendance.data?.find((item) => item.student_id === student.id)
      return <div className="attendance-row" key={student.id}><span>{student.display_name}</span><label><span className="sr-only">Посещаемость: {student.display_name}</span><select value={record?.status ?? ''} onChange={(event) => update.mutate({ studentId: student.id, status: event.target.value as AttendanceStatus, reason: record?.reason ?? undefined, comment: record?.comment ?? undefined })}><option value="">Не отмечено</option>{statuses.map((status) => <option key={status} value={status}>{attendanceNames[status]}</option>)}</select></label>{record && record.status !== 'present' && <input aria-label={`Причина: ${student.display_name}`} placeholder="Причина" defaultValue={record.reason ?? ''} onBlur={(event) => { if (event.target.value !== (record.reason ?? '')) update.mutate({ studentId: student.id, status: record.status, reason: event.target.value, comment: record.comment ?? undefined }) }} />}</div>
    })}</div></QueryState>
  </section>
}
