import { useEffect, useMemo, useState } from 'react'
import type { JournalEntry, Lesson, Student } from '../api/types'
import { attendanceNames, workNames } from '../api/types'
import { formatDate, formatTime } from '../lib/dates'
import { Empty } from '../components/UI'
import { AttendancePanel } from './Attendance'
import { GradeEditor } from './GradeEditor'
import type { GradeConflict, GradeInput } from './useGradeMutation'

export function MobileLesson({ lessons, students, entries, selectedLessonId, onSelectLesson, allowedValues, onSave, isSaving, conflict, onCloseConflict }: {
  lessons: Lesson[]
  students: Student[]
  entries: JournalEntry[]
  selectedLessonId: string
  onSelectLesson: (id: string) => void
  allowedValues: number[]
  onSave: (input: GradeInput, onSaved?: () => void) => void
  isSaving: boolean
  conflict: GradeConflict | null
  onCloseConflict: () => void
}): React.JSX.Element {
  const lesson = lessons.find((item) => item.id === selectedLessonId) ?? lessons[0]
  const works = useMemo(() => {
    const map = new Map<string, JournalEntry>()
    entries.filter((entry) => entry.lesson_id === lesson?.id).forEach((entry) => map.set(entry.lesson_work_id, entry))
    return [...map.values()]
  }, [entries, lesson?.id])
  const [workId, setWorkId] = useState('')
  const [editing, setEditing] = useState<{ entry: JournalEntry; initialValue?: number } | null>(null)
  // Быстрые кнопки сохраняют без редактора: при конфликте открываем его, чтобы локальное значение не пропало молча.
  useEffect(() => { if (conflict) setEditing({ entry: conflict.entry, initialValue: conflict.value }) }, [conflict])
  useEffect(() => { if (works.length && !works.some((work) => work.lesson_work_id === workId)) setWorkId(works[0]!.lesson_work_id) }, [works, workId])
  if (!lesson) return <Empty title="Выберите урок" text="В выбранном классе, предмете и месяце нет уроков." />
  const work = works.find((item) => item.lesson_work_id === workId)
  // Строка журнала ученика по выбранной работе; у ученика без оценки — заготовка без чужих значений.
  const entryFor = (student: Student): JournalEntry | null => !work ? null
    : entries.find((item) => item.student_id === student.id && item.lesson_work_id === work.lesson_work_id)
      ?? { ...work, student_id: student.id, student_name: student.display_name, grade_id: null, grade_value: null, grade_comment: null, grade_version: null }
  const nextStudent = editing ? students[students.findIndex((student) => student.id === editing.entry.student_id) + 1] : undefined
  const nextEntry = nextStudent ? entryFor(nextStudent) : null
  return <div className="mobile-lesson"><section className="mobile-lesson-heading"><label>Урок<select value={lesson.id} onChange={(event) => onSelectLesson(event.target.value)}>{lessons.map((item) => <option key={item.id} value={item.id}>{formatDate(item.lesson_date, { day: 'numeric', month: 'short' })} · {formatTime(item.starts_at)} · {item.topic || item.subject_name}</option>)}</select></label><h2>{lesson.subject_name} · {lesson.class_name}</h2><p>{lesson.topic || 'Тема не указана'}</p></section>
    {works.length > 1 && <label className="field">Работа на уроке<select value={workId} onChange={(event) => setWorkId(event.target.value)}>{works.map((item) => <option key={item.lesson_work_id} value={item.lesson_work_id}>{workNames[item.work_type]} · {item.work_title}</option>)}</select></label>}
    {!work ? <Empty title="На уроке нет работ" text="Добавьте работу кнопкой выше, затем выставьте оценки." /> : <div className="mobile-students">{students.map((student) => {
      const entry = entryFor(student)!
      return <article className="mobile-student" key={student.id}><button className="mobile-student-name" onClick={() => setEditing({ entry })}><strong>{student.display_name}</strong><span>{entry.grade_comment || 'Оценка и комментарий'}</span></button><div className="mobile-grade-buttons" aria-label={`Оценка для ${student.display_name}`}>{allowedValues.map((value) => <button key={value} aria-pressed={entry.grade_value === value} className={entry.grade_value === value ? 'selected' : ''} disabled={isSaving} onClick={() => onSave({ entry, value, comment: entry.grade_comment ?? '' })}>{value}</button>)}</div></article>
    })}</div>}
    <AttendancePanel lessonId={lesson.id} students={students} compact />
    <GradeEditor entry={editing ? entries.find((item) => item.student_id === editing.entry.student_id && item.lesson_work_id === editing.entry.lesson_work_id) ?? editing.entry : null} initialValue={editing?.initialValue} open={Boolean(editing)} onOpenChange={(open) => { if (!open) setEditing(null) }} allowedValues={allowedValues} onSave={onSave} isSaving={isSaving} conflict={conflict} onCloseConflict={onCloseConflict}
      nextName={nextStudent?.display_name} onNext={nextEntry ? () => setEditing({ entry: nextEntry }) : undefined} />
    <p className="muted">Статусы посещаемости: {Object.values(attendanceNames).join(', ')}. Все изменения отправляются на сервер.</p>
  </div>
}
