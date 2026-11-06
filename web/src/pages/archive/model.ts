import type { JournalArchiveSubject } from '../../api/types'

type ArchiveLesson = JournalArchiveSubject['lessons'][number]

/** Пометки посещаемости, как в бумажном журнале. */
export const attendanceMarks: Record<string, string> = { absent: 'н', late: 'оп', excused: 'уп', exempt: 'осв' }

export const attendanceLegend = 'н — отсутствовал, уп — по уважительной причине, оп — опоздал, осв — освобождён'

/** Уроки предмета по месяцам: страница журнала — один месяц, столбец — один урок. */
export function lessonsByMonth(lessons: ArchiveLesson[]): { month: string; lessons: ArchiveLesson[] }[] {
  const months: { month: string; lessons: ArchiveLesson[] }[] = []
  for (const lesson of lessons) {
    const month = lesson.lesson_date.slice(0, 7)
    const last = months[months.length - 1]
    if (last?.month === month) last.lessons.push(lesson)
    else months.push({ month, lessons: [lesson] })
  }
  return months
}

/** Клетка журнала: оценки через пробел и пометка посещаемости, например «5 4» или «н». */
export function cellText(subject: JournalArchiveSubject): (lessonId: string, studentId: string) => string {
  const cells = new Map<string, string>()
  for (const mark of subject.marks) {
    const parts = mark.grades.map((value) => String(value))
    if (mark.attendance) parts.push(attendanceMarks[mark.attendance] ?? mark.attendance)
    cells.set(`${mark.lesson_id}:${mark.student_id}`, parts.join(' '))
  }
  return (lessonId, studentId) => cells.get(`${lessonId}:${studentId}`) ?? ''
}

/** Итоговые оценки ученика по периодам. */
export function termGrade(subject: JournalArchiveSubject): (studentId: string, periodId: string) => string {
  const grades = new Map(subject.term_grades.map((item) => [`${item.student_id}:${item.period_id}`, String(item.value)]))
  return (studentId, periodId) => grades.get(`${studentId}:${periodId}`) ?? ''
}
