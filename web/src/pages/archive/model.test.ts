import { describe, expect, it } from 'vitest'
import type { JournalArchiveSubject } from '../../api/types'
import { cellText, lessonsByMonth, termGrade } from './model'
import { monthRange, substituteTotals } from '../admin/substitutionsModel'

const lesson = (id: string, date: string) => ({ id, lesson_date: date, period_id: 'p1', topic: '', status: 'completed' as const, teacher_name: 'Учитель', homework: '' })

const subject: JournalArchiveSubject = {
  subject_id: 's', subject_name: 'Математика', teachers: ['Учитель'],
  lessons: [lesson('a', '2026-09-30'), lesson('b', '2026-10-01'), lesson('c', '2026-10-02')],
  marks: [
    { lesson_id: 'a', student_id: 'x', grades: [5, 4] },
    { lesson_id: 'b', student_id: 'x', grades: [], attendance: 'absent' },
    { lesson_id: 'c', student_id: 'x', grades: [3], attendance: 'late' },
  ],
  term_grades: [{ student_id: 'x', period_id: 'p1', value: 4 }],
}

describe('journal archive', () => {
  it('splits lessons into months in date order', () => {
    expect(lessonsByMonth(subject.lessons).map((item) => [item.month, item.lessons.map((one) => one.id)])).toEqual([['2026-09', ['a']], ['2026-10', ['b', 'c']]])
  })

  it('writes grades and paper-journal attendance marks into a cell', () => {
    const cell = cellText(subject)
    expect(cell('a', 'x')).toBe('5 4')
    expect(cell('b', 'x')).toBe('н')
    expect(cell('c', 'x')).toBe('3 оп')
    expect(cell('a', 'nobody')).toBe('')
  })

  it('finds term grades by student and period', () => {
    expect(termGrade(subject)('x', 'p1')).toBe('4')
    expect(termGrade(subject)('x', 'p2')).toBe('')
  })
})

describe('substitution log', () => {
  it('covers the whole month, including February of a leap year', () => {
    expect(monthRange('2026-11')).toEqual({ from: '2026-11-01', to: '2026-11-30' })
    expect(monthRange('2028-02')).toEqual({ from: '2028-02-01', to: '2028-02-29' })
  })

  it('counts lessons per substitute, most first', () => {
    const row = (name: string) => ({ teacher_name: name }) as Parameters<typeof substituteTotals>[0][number]
    expect(substituteTotals([row('Петрова'), row('Иванов'), row('Петрова')])).toEqual([{ name: 'Петрова', lessons: 2 }, { name: 'Иванов', lessons: 1 }])
  })
})
