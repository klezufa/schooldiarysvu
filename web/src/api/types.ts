import type { components } from './schema'

type Schema = components['schemas']

export type User = Schema['User']
export type LoginResponse = Schema['LoginResponse']
export type Role = Schema['Role']
export type Student = Schema['Student']
export type Lesson = Schema['Lesson']
export type CurrentLessons = Schema['CurrentLessons']
export type ScheduleEntry = Schema['ScheduleEntry']
export type AcademicYear = Schema['AcademicYear']
export type AcademicPeriod = Schema['AcademicPeriod']
export type NamedEntity = Schema['NamedEntity']
export type JournalEntry = Schema['JournalEntry']
export type LessonWork = Schema['LessonWork']
export type Grade = Schema['Grade']
export type StudentGrade = Schema['StudentGrade']
export type GradeHistoryEntry = Schema['GradeHistoryEntry']
export type Attendance = Schema['Attendance']
export type Homework = Schema['Homework']
export type DiaryFile = Schema['File']
export type Announcement = Schema['Announcement']
export type Notification = Schema['Notification']
export type AuditLog = Schema['AuditLog']
export type GradeScale = Schema['GradeScale']
export type SchoolSettings = Schema['SchoolSettings']
export type Enrollment = Schema['Enrollment']
export type TeachingAssignment = Schema['TeachingAssignment']
export type Assignment = TeachingAssignment
export type LinkRecord = Record<string, unknown> & { id?: string }
export type ReportRecord = Record<string, unknown>
export type AttendanceStatus = NonNullable<Attendance['status']>
export type WorkType = NonNullable<LessonWork['work_type']>

export const roleNames: Record<Role, string> = {
  system_admin: 'Системный администратор',
  administrator: 'Администратор',
  teacher: 'Учитель',
  class_teacher: 'Классный руководитель',
  student: 'Ученик',
  parent: 'Родитель',
  principal: 'Руководитель',
}

/** Системный администратор может всё, что может администратор (проверка прав — на сервере). */
export function isAdministrator(roles: readonly Role[]): boolean {
  return roles.includes('administrator') || roles.includes('system_admin')
}

export const workNames: Record<string, string> = {
  classwork: 'Классная работа', homework: 'Домашняя работа', test: 'Контрольная',
  quiz: 'Проверочная', project: 'Проект', oral: 'Устный ответ',
}

export const attendanceNames: Record<AttendanceStatus, string> = {
  present: 'Присутствует', absent: 'Отсутствует', late: 'Опоздал',
  excused: 'Уважительная причина', exempt: 'Освобождён',
}

export type StudentAnalytics = Schema['StudentAnalytics']
export type TeacherAnalytics = Schema['TeacherAnalytics']
export type SchoolAnalytics = Schema['SchoolAnalytics']
export type SchoolAlert = Schema['SchoolAlert']
export type SystemOverview = Schema['SystemOverview']
export type ServiceStatus = Schema['ServiceStatus']
export type Invitation = Schema['Invitation']
export type InvitationCreated = Schema['InvitationCreated']
export type InvitationLookup = Schema['InvitationLookup']
export type UserImportResult = Schema['UserImportResult']
export type UserImportRow = Schema['UserImportRow']
export type BulkResult = Schema['BulkResult']
export type GenerateLessonsResult = Schema['GenerateLessonsResult']
export type CarryOverResult = Schema['CarryOverResult']

/** Роли, которые обычный администратор может выдавать; административные — только системный администратор. */
export const regularRoles: Role[] = ['teacher', 'class_teacher', 'student', 'parent', 'principal']
export const privilegedRoles: Role[] = ['administrator', 'system_admin']
export type PasswordReset = Schema['PasswordReset']
export type ParentStudentLink = Schema['ParentStudentLink']
export type ChildSuggestion = Schema['ChildSuggestion']
export type OperationsOverview = Schema['OperationsOverview']
