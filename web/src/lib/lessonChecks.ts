import type { Homework, JournalEntry } from '../api/types'

export type LessonCheck = { id: 'attendance' | 'grades' | 'homework'; tone: 'ok' | 'warn' | 'info'; title: string; text: string }

/** Памятка перед завершением урока. Ничего не блокирует: учитель может сохранить урок с любым результатом. */
export function buildLessonChecks({ lessonId, students, attendance, entries, homework }: {
  lessonId: string; students: number; attendance: number; entries: JournalEntry[]; homework: Homework[]
}): LessonCheck[] {
  const own = entries.filter((entry) => entry.lesson_id === lessonId)
  const missing = own.filter((entry) => entry.grade_value === null || entry.grade_value === undefined).length
  const linked = homework.filter((item) => item.lesson_id === lessonId)
  const attendanceCheck: LessonCheck = attendance === 0
    ? { id: 'attendance', tone: 'warn', title: 'Посещаемость', text: 'Не заполнена ни у одного ученика.' }
    : attendance < students
      ? { id: 'attendance', tone: 'warn', title: 'Посещаемость', text: `Отмечено ${attendance} из ${students} учеников.` }
      : { id: 'attendance', tone: 'ok', title: 'Посещаемость', text: `Отмечены все ученики (${students}).` }
  const gradesCheck: LessonCheck = own.length === 0
    ? { id: 'grades', tone: 'info', title: 'Оценки', text: 'На уроке нет работ с оценками — проверять нечего.' }
    : missing === 0
      ? { id: 'grades', tone: 'ok', title: 'Оценки', text: 'Во всех ячейках работ урока есть оценки.' }
      : { id: 'grades', tone: 'warn', title: 'Оценки', text: `Без оценки ${missing} из ${own.length} ячеек. Это допустимо, если работа не оценивается.` }
  const homeworkCheck: LessonCheck = linked.length === 0
    ? { id: 'homework', tone: 'info', title: 'Домашнее задание', text: 'Не выдано. Это не мешает сохранению урока.' }
    : linked.every((item) => item.status === 'draft')
      ? { id: 'homework', tone: 'warn', title: 'Домашнее задание', text: 'Есть только неопубликованный черновик — ученики его не видят.' }
      : { id: 'homework', tone: 'ok', title: 'Домашнее задание', text: 'Опубликовано.' }
  return [attendanceCheck, gradesCheck, homeworkCheck]
}
