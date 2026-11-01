import type { UserImportRow } from '../../api/types'

const roleAliases: Record<string, UserImportRow['role']> = {
  student: 'student', ученик: 'student', ученица: 'student', учащийся: 'student',
  teacher: 'teacher', учитель: 'teacher', преподаватель: 'teacher',
  parent: 'parent', родитель: 'parent', мама: 'parent', папа: 'parent',
  principal: 'principal', руководитель: 'principal', директор: 'principal', завуч: 'principal',
}

export type Parsed = { rows: UserImportRow[]; problems: { line: number; message: string }[] }

/**
 * Разбирает таблицу, вставленную из Excel (табуляция) или CSV (; или ,).
 * Колонки: ФИО; роль; класс; логин. Заголовок распознаётся и пропускается; роль по умолчанию — ученик.
 */
export function parseImport(text: string, defaultRole: UserImportRow['role'] = 'student'): Parsed {
  const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
  const result: Parsed = { rows: [], problems: [] }
  lines.forEach((line, index) => {
    const separator = line.includes('\t') ? '\t' : line.includes(';') ? ';' : ','
    const cells = line.split(separator).map((cell) => cell.trim().replace(/^"(.*)"$/, '$1'))
    const [name = '', roleText = '', className = '', login = ''] = cells
    if (index === 0 && /фио|имя|name/i.test(name)) return
    if (!name) return
    const role = roleText ? roleAliases[roleText.toLowerCase()] : defaultRole
    if (!role) {
      result.problems.push({ line: index + 1, message: `Неизвестная роль «${roleText}»` })
      return
    }
    result.rows.push({ display_name: name, role, class_name: className || undefined, login: login || undefined })
  })
  return result
}

