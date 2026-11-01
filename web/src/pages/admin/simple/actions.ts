import { ApiError, mutation } from '../../../api/client'
import type { AcademicPeriod, Assignment, Role, ScheduleEntry, User } from '../../../api/types'
import { roleNames } from '../../../api/types'
import { addDays, dateISO } from '../../../lib/dates'
import { randomPassword, suggestLogin } from '../helpers'
import { bells } from '../scheduleSlots'
import { lessonRange, loginCandidates } from './setup'

const roleLabel = (roles: Role[]) => (roles[0] ? roleNames[roles[0]] : '')

/** Логин и временный пароль, которые администратор передаёт человеку. */
export type Credential = { role: string; name: string; login: string; password: string }

/**
 * Создаёт пользователя с логином из ФИО. Если логин занят (однофамилец с теми же инициалами), пробует следующий
 * вариант с номером — администратору не нужно придумывать логины.
 */
export async function createPerson(name: string, roles: Role[], classId?: string): Promise<{ user: User; credential: Credential }> {
  const password = randomPassword()
  for (const login of loginCandidates(suggestLogin(name))) {
    try {
      const user = await mutation<User>('/users', 'POST', { login, password, display_name: name.trim(), roles, class_id: classId || undefined })
      return { user, credential: { role: roleLabel(roles), name: user.display_name, login: user.login, password } }
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 409) continue
      throw caught
    }
  }
  throw new Error('Не удалось подобрать свободный логин. Создайте пользователя в расширенном режиме и задайте логин вручную.')
}

/** Новый временный пароль: показывается один раз, при входе человек сменит его на свой. */
export async function resetPassword(user: User): Promise<Credential> {
  const result = await mutation<{ password: string }>(`/users/${user.id}/password-reset`, 'POST')
  return { role: roleLabel(user.roles), name: user.display_name, login: user.login, password: result.password }
}

/** Связывает родителя с ребёнком с сегодняшнего дня. */
export async function linkParent(parent: User, student: User): Promise<void> {
  if (!parent.parent_id || !student.student_id) throw new Error('Не удалось связать родителя с ребёнком')
  await mutation('/parent-student', 'POST', { parent_id: parent.parent_id, student_id: student.student_id, valid_from: dateISO() })
}

function activeAssignment(assignments: Assignment[], teacherId: string, classId: string, subjectId: string, periodId: string): Assignment | undefined {
  const today = dateISO()
  return assignments.find((item) => item.teacher_id === teacherId && item.class_id === classId && item.subject_id === subjectId
    && item.academic_period_id === periodId && (!item.ends_on || item.ends_on >= today))
}

/**
 * Назначение «учитель — класс — предмет» — техническая подробность: в упрощённом режиме оно создаётся само, когда
 * администратор указывает, кто ведёт предмет. Новое действует с начала периода. Возвращает дату начала назначения:
 * строка шаблона не может начаться раньше неё.
 */
export async function ensureAssignment(assignments: Assignment[], period: AcademicPeriod, teacherId: string, classId: string, subjectId: string): Promise<string> {
  const existing = activeAssignment(assignments, teacherId, classId, subjectId, period.id)
  if (existing) return existing.starts_on
  await mutation('/teaching-assignments', 'POST', { teacher_id: teacherId, class_id: classId, subject_id: subjectId, academic_period_id: period.id, starts_on: period.starts_on })
  return period.starts_on
}

/** Снимает учителя с предмета в классе: назначение завершается сегодня (не раньше своего начала). */
export async function endAssignment(assignment: Assignment): Promise<void> {
  const today = dateISO()
  await mutation(`/teaching-assignments/${assignment.id}`, 'PATCH', { ends_on: assignment.starts_on > today ? assignment.starts_on : today })
}

/**
 * Классный руководитель: связь с классом и роль «классный руководитель» (без неё у учителя нет раздела «Мой класс»).
 * Прежний руководитель класса, если он есть, завершает работу накануне.
 */
export async function setClassTeacher(teacher: User, classId: string, current?: { id: string; starts_on: string }): Promise<void> {
  if (!teacher.teacher_id) throw new Error('Классным руководителем может быть только учитель')
  let startsOn = dateISO()
  if (current) {
    const endsOn = current.starts_on > addDays(startsOn, -1) ? current.starts_on : addDays(startsOn, -1)
    await mutation(`/class-teachers/${current.id}`, 'PATCH', { ends_on: endsOn })
    startsOn = addDays(endsOn, 1)
  }
  await mutation('/class-teachers', 'POST', { teacher_id: teacher.teacher_id, class_id: classId, starts_on: startsOn })
  if (!teacher.roles.includes('class_teacher')) {
    await mutation(`/users/${teacher.id}`, 'PATCH', { roles: [...teacher.roles, 'class_teacher'] })
  }
}

/** Создаёт уроки класса по шаблону с сегодняшнего дня до конца периода; повторный вызов ничего не дублирует. */
export async function generateClassLessons(period: AcademicPeriod, classId: string): Promise<void> {
  const range = lessonRange(period, dateISO())
  if (!range) return
  await mutation('/schedule/generate', 'POST', { ...range, class_id: classId })
}

export type LessonSlot = { weekday: number; number: number }
export type LessonChoice = { subjectId: string; teacherId: string; roomId: string }

/**
 * Ставит урок в сетку: создаёт назначение учителя, если его нет, сохраняет строку шаблона на весь период и сразу
 * создаёт уроки — в упрощённом режиме нет отдельного шага «создать уроки по шаблону». Предмет строки сервер не меняет,
 * поэтому при смене предмета строка пересоздаётся (её будущие незаполненные уроки сервер убирает сам).
 */
export async function saveLesson(assignments: Assignment[], period: AcademicPeriod, classId: string, slot: LessonSlot, choice: LessonChoice, existing?: ScheduleEntry): Promise<void> {
  const assignedFrom = await ensureAssignment(assignments, period, choice.teacherId, classId, choice.subjectId)
  const [startsAt, endsAt] = bells[slot.number - 1] ?? ['15:15', '16:00']
  if (existing && existing.subject_id === choice.subjectId) {
    await mutation(`/schedule-entries/${existing.id}`, 'PATCH', {
      teacher_id: choice.teacherId, room_id: choice.roomId || null, weekday: existing.weekday, lesson_number: existing.lesson_number,
      starts_at: existing.starts_at, ends_at: existing.ends_at, valid_from: existing.valid_from, valid_to: existing.valid_to ?? null, week_parity: existing.week_parity ?? 0,
    })
  } else {
    if (existing) await mutation(`/schedule-entries/${existing.id}`, 'DELETE')
    await mutation('/schedule-entries', 'POST', {
      class_id: classId, subject_id: choice.subjectId, teacher_id: choice.teacherId, room_id: choice.roomId || undefined, academic_period_id: period.id,
      weekday: slot.weekday, lesson_number: slot.number, starts_at: startsAt, ends_at: endsAt, valid_from: assignedFrom > period.starts_on ? assignedFrom : period.starts_on, week_parity: 0,
    })
  }
  await generateClassLessons(period, classId)
}

export async function removeLesson(entry: ScheduleEntry): Promise<void> {
  await mutation(`/schedule-entries/${entry.id}`, 'DELETE')
}
