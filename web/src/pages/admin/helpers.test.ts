import { parseImport } from './importParser'
import { randomPassword, suggestLogin } from './helpers'
import { distributionParts, dueLabel, firstName, previousPeriod } from '../dashboards/helpers'
import { addDays, dateISO } from '../../lib/dates'

describe('import parser', () => {
  it('reads Excel paste (tabs), skips the header and applies the default role', () => {
    const parsed = parseImport('ФИО\tроль\tкласс\nИванов Иван Петрович\t\t5А\nПетрова Анна\tучитель\t', 'student')
    expect(parsed.problems).toEqual([])
    expect(parsed.rows).toEqual([
      { display_name: 'Иванов Иван Петрович', role: 'student', class_name: '5А', login: undefined },
      { display_name: 'Петрова Анна', role: 'teacher', class_name: undefined, login: undefined },
    ])
  })

  it('reads semicolon CSV with quotes and reports unknown roles by line', () => {
    const parsed = parseImport('"Сидоров Олег";ученик;5Б;sidorov\nКто-то;директор школы\n\n', 'student')
    expect(parsed.rows).toEqual([{ display_name: 'Сидоров Олег', role: 'student', class_name: '5Б', login: 'sidorov' }])
    expect(parsed.problems).toEqual([{ line: 2, message: 'Неизвестная роль «директор школы»' }])
  })
})

describe('admin helpers', () => {
  it.each([
    ['Иванов Иван Петрович', 'ivanov.ip'],
    ['Щукина Юлия', 'shchukina.y'],
    ['Ёлкин', 'elkin'],
    ['', ''],
  ])('suggests login %s → %s like the server import does', (name, login) => {
    expect(suggestLogin(name)).toBe(login)
  })

  it('generates readable temporary passwords of at least 12 characters', () => {
    const password = randomPassword()
    expect(password.length).toBeGreaterThanOrEqual(12)
    expect(password).not.toMatch(/[01OIl]/)
  })
})

describe('dashboard helpers', () => {
  it('takes the given name from «Фамилия Имя Отчество»', () => {
    expect(firstName('Иванов Иван Петрович')).toBe('Иван')
    expect(firstName('Админ')).toBe('Админ')
    expect(firstName(undefined)).toBe('')
  })

  it('labels deadlines relative to today', () => {
    const today = dateISO()
    expect(dueLabel(today).text).toBe('Сегодня')
    expect(dueLabel(addDays(today, 1)).text).toBe('Завтра')
    expect(dueLabel(addDays(today, -1)).tone).toBe('danger')
  })

  it('orders grade distribution from best to worst with grade names', () => {
    const parts = distributionParts([{ value: 3, count: 4 }, { value: 5, count: 7 }])
    expect(parts.map((part) => part.key)).toEqual(['5', '3'])
    expect(parts[0]!.label).toBe('«5» отлично')
  })

  it('suggests the most recent finished period', () => {
    const periods = [
      { id: 'q1', starts_on: '2026-09-01', ends_on: '2026-10-31' },
      { id: 'q2', starts_on: '2026-11-01', ends_on: '2026-12-31' },
    ]
    expect(previousPeriod(periods, '2026-11-01')?.id).toBe('q1')
    expect(previousPeriod(periods, '2026-10-15')).toBeUndefined()
  })
})
