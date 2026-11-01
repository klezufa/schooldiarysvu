import { lessonRange, loginCandidates, setupSteps, type SetupInput } from './setup'

const ready: SetupInput = { hasCurrentPeriod: true, classes: 2, subjects: 5, teachers: 3, students: 40, classesWithoutSchedule: [] }

describe('setup steps', () => {
  it('marks every step done for a prepared school', () => {
    expect(setupSteps(ready).every((step) => step.done)).toBe(true)
  })

  it('keeps the dependency order and points the schedule step at the first class without lessons', () => {
    const steps = setupSteps({ ...ready, classesWithoutSchedule: [{ id: 'c2', name: '5Б' }, { id: 'c3', name: '6А' }] })
    expect(steps.map((step) => step.id)).toEqual(['period', 'classes', 'subjects', 'teachers', 'students', 'schedule'])
    const schedule = steps.find((step) => step.id === 'schedule')!
    expect(schedule.done).toBe(false)
    expect(schedule.classId).toBe('c2')
    expect(schedule.text).toContain('5Б, 6А')
  })

  it('does not report the schedule as done while there are no classes', () => {
    const steps = setupSteps({ ...ready, classes: 0 })
    expect(steps.find((step) => step.id === 'classes')!.done).toBe(false)
    expect(steps.find((step) => step.id === 'schedule')!.done).toBe(false)
  })

  it('shortens a long list of classes without a schedule', () => {
    const many = Array.from({ length: 6 }, (_, index) => ({ id: `c${index}`, name: `${index + 5}А` }))
    expect(setupSteps({ ...ready, classesWithoutSchedule: many }).find((step) => step.id === 'schedule')!.text).toContain('и ещё 2')
  })
})

describe('lesson range after a schedule change', () => {
  const period = { starts_on: '2026-11-09', ends_on: '2026-12-27' }

  it('starts today in the middle of a period and ends with the period', () => {
    expect(lessonRange(period, '2026-11-20')).toEqual({ from: '2026-11-20', to: '2026-12-27' })
  })

  it('starts with the period when it has not begun yet', () => {
    expect(lessonRange(period, '2026-11-02')).toEqual({ from: '2026-11-09', to: '2026-12-27' })
  })

  it('returns nothing for a finished period', () => {
    expect(lessonRange(period, '2027-01-10')).toBeNull()
  })

  it('stays within the 120 days the server generates at once', () => {
    expect(lessonRange({ starts_on: '2026-09-01', ends_on: '2027-05-31' }, '2026-09-01')).toEqual({ from: '2026-09-01', to: '2026-12-29' })
  })
})

describe('login candidates', () => {
  it('tries the suggested login first, then numbered variants', () => {
    const candidates = loginCandidates('ivanov.ip')
    expect(candidates[0]).toBe('ivanov.ip')
    expect(candidates[1]).toBe('ivanov.ip2')
    expect(new Set(candidates).size).toBe(candidates.length)
  })

  it('pads a login that is too short for the server', () => {
    expect(loginCandidates('li')[0]).toBe('li.user')
    expect(loginCandidates('')[0]).toBe('user.user')
  })
})
