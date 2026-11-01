import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CheckCircle2, Info } from 'lucide-react'
import { api, queryString } from '../api/client'
import type { Attendance, Homework, JournalEntry, Lesson, Student } from '../api/types'
import { journalKey } from '../journal/useGradeMutation'
import { scopedKey } from '../lib/student'
import { buildLessonChecks } from '../lib/lessonChecks'
import { useStudent } from '../studentContext'

const icons = { ok: CheckCircle2, warn: AlertTriangle, info: Info } as const
const toneLabels = { ok: 'Готово', warn: 'Проверьте', info: 'К сведению' } as const

export function LessonCompletionCheck({ lesson }: { lesson: Lesson }): React.JSX.Element {
  const { studentId } = useStudent()
  const attendance = useQuery({ queryKey: ['attendance', lesson.id], queryFn: () => api<Attendance[]>(`/attendance?lesson_id=${lesson.id}`) })
  const students = useQuery({ queryKey: ['class-students', lesson.class_id, lesson.lesson_date], queryFn: () => api<Student[]>(`/classes/${lesson.class_id}/students?date=${lesson.lesson_date}`) })
  const journal = useQuery({ queryKey: journalKey(lesson.class_id, lesson.subject_id, lesson.academic_period_id), queryFn: () => api<JournalEntry[]>(`/journal${queryString({ class_id: lesson.class_id, subject_id: lesson.subject_id, academic_period_id: lesson.academic_period_id })}`) })
  const homework = useQuery({ queryKey: scopedKey('homework', studentId), queryFn: () => api<Homework[]>(`/homework${queryString({ student_id: studentId })}`) })
  if ([attendance, students, journal, homework].some((query) => query.isLoading)) return <p className="muted" role="status">Проверяем заполненность урока…</p>
  if ([attendance, students, journal, homework].some((query) => query.error)) return <p className="muted" role="status">Не удалось проверить заполненность урока. Сохранение урока это не блокирует.</p>
  const checks = buildLessonChecks({ lessonId: lesson.id, students: students.data?.length ?? 0, attendance: attendance.data?.length ?? 0, entries: journal.data ?? [], homework: homework.data ?? [] })
  return <section className="completion-check-list" aria-label="Проверка перед завершением урока"><h4>Перед завершением урока</h4><ul>{checks.map((check) => {
    const Icon = icons[check.tone]
    return <li key={check.id} className={`check-${check.tone}`}><Icon aria-hidden="true" /><div><strong>{check.title}: {toneLabels[check.tone]}</strong><span>{check.text}</span></div></li>
  })}</ul><p className="muted">Проверка носит справочный характер и не блокирует сохранение.</p></section>
}
