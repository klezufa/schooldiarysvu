import { buildLessonChecks } from '../lib/lessonChecks'
import { entryFixture, homeworkFixture } from '../test/fixtures'

const base = { lessonId: entryFixture.lesson_id, students: 2, attendance: 2, entries: [entryFixture], homework: [{ ...homeworkFixture, lesson_id: entryFixture.lesson_id }] }
const tones = (checks: ReturnType<typeof buildLessonChecks>) => Object.fromEntries(checks.map((check) => [check.id, check.tone]))

describe('lesson completion check', () => {
  it('reports everything ready when attendance, grades and homework are complete', () => {
    expect(tones(buildLessonChecks(base))).toEqual({ attendance: 'ok', grades: 'ok', homework: 'ok' })
  })

  it('warns about partial attendance, empty grade cells and unpublished homework', () => {
    const checks = buildLessonChecks({ ...base, attendance: 1, entries: [{ ...entryFixture, grade_value: null }], homework: [{ ...homeworkFixture, lesson_id: entryFixture.lesson_id, status: 'draft' }] })
    expect(tones(checks)).toEqual({ attendance: 'warn', grades: 'warn', homework: 'warn' })
    expect(checks[0]?.text).toBe('Отмечено 1 из 2 учеников.')
  })

  it('treats missing homework and missing works as informational, never as errors', () => {
    const checks = buildLessonChecks({ ...base, entries: [], homework: [] })
    expect(tones(checks)).toMatchObject({ grades: 'info', homework: 'info' })
  })

  it('ignores entries and homework that belong to other lessons', () => {
    const checks = buildLessonChecks({ ...base, entries: [{ ...entryFixture, lesson_id: 'other', grade_value: null }], homework: [{ ...homeworkFixture, lesson_id: 'other' }] })
    expect(tones(checks)).toMatchObject({ grades: 'info', homework: 'info' })
  })

  it('flags an empty attendance sheet', () => {
    expect(buildLessonChecks({ ...base, attendance: 0 })[0]?.text).toContain('Не заполнена')
  })
})
