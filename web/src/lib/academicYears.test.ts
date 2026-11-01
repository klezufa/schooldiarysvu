import { describe, expect, it } from 'vitest'
import type { AcademicPeriod, AcademicYear, NamedEntity } from '../api/types'
import { classLabeler, classesOfYear, defaultDateIn, openClasses, pickCurrentPeriod, previousPeriod } from './academicYears'

const years: AcademicYear[] = [
  { id: 'y26', name: '2026/2027', starts_on: '2026-09-01', ends_on: '2027-05-31', is_current: true },
  { id: 'y27', name: '2027/2028', starts_on: '2027-09-01', ends_on: '2028-05-31', is_current: false },
  { id: 'y24', name: '2024/2025', starts_on: '2024-09-01', ends_on: '2025-05-31', is_current: false },
]
const classes: NamedEntity[] = [
  { id: 'c5a', name: '5А', academic_year_id: 'y26' },
  { id: 'c5a-next', name: '5А', academic_year_id: 'y27' },
  { id: 'c6a', name: '6А', academic_year_id: 'y27' },
  { id: 'c9b', name: '9Б', academic_year_id: 'y24' },
]
const period = (id: string, starts_on: string, ends_on: string, academic_year_id = 'y26'): AcademicPeriod =>
  ({ id, name: id, starts_on, ends_on, academic_year_id, status: 'open', allow_late_edits: false })
const periods = [
  period('II', '2026-11-09', '2026-12-28'),
  period('I', '2026-09-01', '2026-10-25'),
  period('III', '2027-01-11', '2027-03-20'),
  period('next-I', '2027-09-01', '2027-10-25', 'y27'),
]

describe('classLabeler', () => {
  it('adds the year only when classes of several years are listed', () => {
    expect(classLabeler(classes, years)(classes[0]!)).toBe('5А · 2026/2027')
    expect(classLabeler(classes, years)(classes[1]!)).toBe('5А · 2027/2028')
    const single = classesOfYear(classes, 'y27')
    expect(classLabeler(single, years)(single[1]!)).toBe('6А')
  })
})

describe('openClasses', () => {
  it('keeps the current and the prepared next year, drops finished years', () => {
    expect(openClasses(classes, years, '2026-11-01').map((item) => item.id)).toEqual(['c5a', 'c5a-next', 'c6a'])
  })
})

describe('pickCurrentPeriod', () => {
  it('returns the running period', () => {
    expect(pickCurrentPeriod(periods, '2026-11-20')?.id).toBe('II')
  })
  it('returns the last started period during holidays, not the first one', () => {
    expect(pickCurrentPeriod(periods, '2027-01-03')?.id).toBe('II')
    expect(pickCurrentPeriod(periods, '2027-07-15')?.id).toBe('III')
  })
  it('returns the first period before the year starts', () => {
    expect(pickCurrentPeriod(periods, '2026-08-20')?.id).toBe('I')
  })
})

describe('previousPeriod', () => {
  it('crosses the year boundary', () => {
    expect(previousPeriod(periods, periods[3])?.id).toBe('III')
    expect(previousPeriod(periods, periods[0])?.id).toBe('I')
    expect(previousPeriod(periods, periods[1])).toBeUndefined()
  })
})

describe('defaultDateIn', () => {
  it('uses today inside the period and the first day otherwise', () => {
    expect(defaultDateIn({ starts_on: '2026-11-09', ends_on: '2026-12-28' }, '2026-11-20')).toBe('2026-11-20')
    expect(defaultDateIn({ starts_on: '2027-09-01', ends_on: '2027-10-25' }, '2026-11-20')).toBe('2027-09-01')
  })
})
