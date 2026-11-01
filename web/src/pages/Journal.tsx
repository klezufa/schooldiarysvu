import { useEffect, useMemo, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { Expand, Minimize, Plus, Undo2 } from 'lucide-react'
import { api, mutation, queryString } from '../api/client'
import type { JournalEntry, Lesson, Student } from '../api/types'
import { workNames } from '../api/types'
import { useAcademic } from '../hooks/useAcademic'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { dateISO } from '../lib/dates'
import { PageHeader, QueryState, StateView } from '../components/UI'
import { useToast } from '../components/toastContext'
import { JournalTable, type JournalMode } from '../journal/JournalTable'
import { MobileLesson } from '../journal/MobileLesson'
import { AttendancePanel } from '../journal/Attendance'
import { journalKey, useGradeMutation, type GradeInput } from '../journal/useGradeMutation'

const workSchema = z.object({ lesson_id: z.string().uuid('Выберите урок'), work_type: z.enum(['classwork', 'homework', 'test', 'quiz', 'project', 'oral']), title: z.string().trim().min(1, 'Введите название').max(300), weight: z.coerce.number().positive().max(100), position: z.coerce.number().int().min(1).max(100) })
type WorkInput = z.infer<typeof workSchema>

export function JournalPage(): React.JSX.Element {
  const location = useLocation()
  const initialLesson = (location.state as { lesson?: Lesson } | null)?.lesson
  const { classes, subjects, periods, scales, periodId, setPeriodId } = useAcademic()
  const [classId, setClassId] = useState(initialLesson?.class_id ?? '')
  const [subjectId, setSubjectId] = useState(initialLesson?.subject_id ?? '')
  const [month, setMonth] = useState(initialLesson?.lesson_date.slice(0, 7) ?? dateISO().slice(0, 7))
  const [mode, setMode] = useState<JournalMode>(initialLesson ? 'lesson' : 'all')
  const [lessonId, setLessonId] = useState(initialLesson?.id ?? '')
  const [studentId, setStudentId] = useState('')
  const [compact, setCompact] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [workOpen, setWorkOpen] = useState(false)
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  useEffect(() => { if (initialLesson) setPeriodId(initialLesson.academic_period_id) }, [initialLesson, setPeriodId])
  const journal = useQuery({ queryKey: journalKey(classId, subjectId, periodId), queryFn: () => api<JournalEntry[]>(`/journal${queryString({ class_id: classId, subject_id: subjectId, academic_period_id: periodId })}`), enabled: Boolean(classId && subjectId && periodId) })
  const students = useQuery({ queryKey: ['class-students', classId, dateISO()], queryFn: () => api<Student[]>(`/classes/${classId}/students?date=${dateISO()}`), enabled: Boolean(classId) })
  const period = periods.data?.find((item) => item.id === periodId)
  const schedule = useQuery({ queryKey: ['schedule', 'journal', periodId], queryFn: () => api<Lesson[]>(`/schedule?from=${period?.starts_on}&to=${period?.ends_on}`), enabled: Boolean(period) })
  // Учитель видит только свои классы и предметы: варианты строятся из его расписания за период,
  // а не из всех справочников (иначе по умолчанию открывается чужой, пустой журнал).
  const teaching = useMemo(() => {
    const classMap = new Map<string, string>()
    const subjectsByClass = new Map<string, Map<string, string>>()
    for (const lesson of schedule.data ?? []) {
      classMap.set(lesson.class_id, lesson.class_name ?? lesson.class_id)
      const own = subjectsByClass.get(lesson.class_id) ?? new Map<string, string>()
      own.set(lesson.subject_id, lesson.subject_name ?? lesson.subject_id)
      subjectsByClass.set(lesson.class_id, own)
    }
    return { classMap, subjectsByClass }
  }, [schedule.data])
  const classOptions = useMemo(() => teaching.classMap.size ? [...teaching.classMap].map(([id, name]) => ({ id, name })) : (classes.data ?? []), [teaching, classes.data])
  const subjectOptions = useMemo(() => { const own = teaching.subjectsByClass.get(classId); return own ? [...own].map(([id, name]) => ({ id, name })) : (subjects.data ?? []) }, [teaching, classId, subjects.data])
  useEffect(() => { if (classOptions.length && !classOptions.some((item) => item.id === classId)) setClassId(classOptions[0]!.id) }, [classId, classOptions])
  useEffect(() => { if (subjectOptions.length && !subjectOptions.some((item) => item.id === subjectId)) setSubjectId(subjectOptions[0]!.id) }, [subjectId, subjectOptions])
  const lessons = useMemo(() => (schedule.data ?? []).filter((lesson) => lesson.class_id === classId && lesson.subject_id === subjectId && lesson.lesson_date.startsWith(month)), [schedule.data, classId, subjectId, month])
  useEffect(() => { if (lessons.length && !lessons.some((lesson) => lesson.id === lessonId)) setLessonId(lessons[0]!.id) }, [lessons, lessonId])
  const allowedValues = scales.data?.find((scale) => scale.is_active)?.allowed_values ?? [2, 3, 4, 5]
  const grades = useGradeMutation(classId, subjectId, periodId)
  const saveGrade = (input: GradeInput, onSaved?: () => void) => grades.save.mutate(input, { onSuccess: onSaved })
  const mobile = useMediaQuery('(max-width: 720px)')
  const closeConflict = () => { void grades.reloadServer(); grades.setConflict(null) }
  const { register, handleSubmit, reset, formState: { errors } } = useForm<z.input<typeof workSchema>, unknown, WorkInput>({ resolver: zodResolver(workSchema), defaultValues: { lesson_id: initialLesson?.id ?? '', work_type: 'classwork', title: 'Классная работа', weight: 1, position: 1 } })
  useEffect(() => { if (lessonId) reset((values) => ({ ...values, lesson_id: lessonId })) }, [lessonId, reset])
  const createWork = useMutation({
    mutationFn: (values: WorkInput) => mutation('/lesson-works', 'POST', values),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['journal', classId, subjectId, periodId] }); setWorkOpen(false); showToast('Работа добавлена в журнал') },
    onError: (error) => showToast(error.message, 'error'),
  })
  const selectedLesson = lessons.find((lesson) => lesson.id === lessonId)
  const loading = journal.isLoading || students.isLoading || classes.isLoading || subjects.isLoading
  const error = journal.error ?? students.error ?? classes.error ?? subjects.error
  return <div className={fullscreen ? 'journal-fullscreen' : ''}><PageHeader eyebrow="Рабочая таблица" title="Журнал" description="Оценки, работы и посещаемость. Изменения сохраняются с проверкой версии." actions={<><button className="button secondary" onClick={() => setCompact((value) => !value)}>{compact ? 'Комфортный режим' : 'Компактный режим'}</button><button className="icon-button desktop-only" onClick={() => setFullscreen((value) => !value)} aria-label={fullscreen ? 'Выйти из полноэкранного режима' : 'Полноэкранный режим'}>{fullscreen ? <Minimize /> : <Expand />}</button></>} />
    <section className="journal-filters" aria-label="Фильтры журнала"><label>Класс<select value={classId} onChange={(event) => { setClassId(event.target.value); setLessonId('') }}>{classOptions.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label><label>Предмет<select value={subjectId} onChange={(event) => { setSubjectId(event.target.value); setLessonId('') }}>{subjectOptions.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label><label>Период<select value={periodId} onChange={(event) => setPeriodId(event.target.value)}>{periods.data?.map((item) => <option key={item.id} value={item.id}>{item.name}{item.status === 'closed' ? ' (закрыт)' : ''}</option>)}</select></label><label>Месяц<input type="month" value={month} onChange={(event) => setMonth(event.target.value)} /></label><button className="button secondary" onClick={() => setMonth(dateISO().slice(0, 7))}>Текущий месяц</button></section>
    <div className="journal-toolbar"><div className="segmented" aria-label="Режим журнала"><button className={mode === 'lesson' ? 'active' : ''} onClick={() => setMode('lesson')}>По уроку</button><button className={mode === 'student' ? 'active' : ''} onClick={() => setMode('student')}>По ученику</button><button className={mode === 'all' ? 'active' : ''} onClick={() => setMode('all')}>Весь журнал</button></div>{mode === 'lesson' && <label>Урок<select value={lessonId} onChange={(event) => setLessonId(event.target.value)}>{lessons.map((lesson) => <option key={lesson.id} value={lesson.id}>{lesson.lesson_date} · {lesson.topic || 'Без темы'}</option>)}</select></label>}{mode === 'student' && <label>Ученик<select value={studentId} onChange={(event) => setStudentId(event.target.value)}><option value="">Выберите ученика</option>{students.data?.map((student) => <option key={student.id} value={student.id}>{student.display_name}</option>)}</select></label>}<button className="button primary" disabled={!lessonId} onClick={() => setWorkOpen((value) => !value)}><Plus />Добавить работу</button><button className="button secondary" disabled={!grades.canUndo || grades.undo.isPending} onClick={() => grades.undo.mutate()}><Undo2 />Отменить изменение</button></div>
    {workOpen && <form className="inline-create panel" onSubmit={handleSubmit((values) => createWork.mutate(values))}><h2>Работа на уроке</h2><input type="hidden" {...register('lesson_id')} />{errors.lesson_id && <p className="field-error">{errors.lesson_id.message}</p>}<label>Тип<select {...register('work_type')}>{Object.entries(workNames).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label><label>Название<input {...register('title')} />{errors.title && <small className="field-error">{errors.title.message}</small>}</label><label>Вес<input type="number" step="0.1" {...register('weight')} /></label><label>Позиция<input type="number" {...register('position')} /></label><button className="button primary" disabled={createWork.isPending}>Создать работу</button></form>}
    {grades.save.error && !grades.conflict && <StateView error={grades.save.error} />}
    <QueryState isLoading={loading} error={error} retry={() => { journal.refetch(); students.refetch() }} isEmpty={!students.data?.length} emptyTitle="В классе нет учеников" emptyText="Проверьте зачисления на выбранную дату.">
      {mobile
        ? <div className="mobile-journal"><MobileLesson lessons={lessons} students={students.data ?? []} entries={journal.data ?? []} selectedLessonId={lessonId} onSelectLesson={setLessonId} allowedValues={allowedValues} onSave={saveGrade} isSaving={grades.save.isPending} conflict={grades.conflict} onCloseConflict={closeConflict} /></div>
        : <div className="desktop-journal"><JournalTable entries={journal.data ?? []} students={students.data ?? []} allowedValues={allowedValues} onSave={saveGrade} isSaving={grades.save.isPending} conflict={grades.conflict} onCloseConflict={closeConflict} month={month} mode={mode} selectedLessonId={lessonId} selectedStudentId={studentId} compact={compact} />{selectedLesson && <AttendancePanel lessonId={selectedLesson.id} students={students.data ?? []} />}</div>}
    </QueryState>
    <p className="keyboard-help">Клавиатура: цифры — выбрать оценку, стрелки — перемещаться, Enter — открыть/сохранить, Escape — закрыть. Значок * — есть комментарий.</p>
  </div>
}
