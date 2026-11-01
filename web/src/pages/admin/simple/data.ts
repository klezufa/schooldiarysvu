import { useQuery } from '@tanstack/react-query'
import { api } from '../../../api/client'
import type { Assignment, SchoolAnalytics, ScheduleEntry } from '../../../api/types'
import { dateISO } from '../../../lib/dates'
import { useDirectory, usePeople } from '../helpers'

export type ClassTeacherLink = { id: string; teacher_id: string; class_id: string; starts_on: string; ends_on?: string | null }

/**
 * Данные, которые нужны почти каждому экрану упрощённого режима: текущий учебный год и четверть, его классы, учителя,
 * назначения и классные руководители. Ключи кэша общие с расширенным режимом.
 */
export function useSimpleData() {
  const directory = useDirectory()
  const teachers = usePeople('teacher')
  const assignments = useQuery({ queryKey: ['teaching-assignments'], queryFn: () => api<Assignment[]>('/teaching-assignments') })
  const classTeachers = useQuery({ queryKey: ['class-teachers'], queryFn: () => api<ClassTeacherLink[]>('/class-teachers') })
  const period = directory.currentPeriod
  const classes = directory.classesOfYear(directory.year?.id).slice().sort((a, b) => a.name.localeCompare(b.name, 'ru', { numeric: true }))
  const today = dateISO()
  const activeClassTeacher = (classId: string) => classTeachers.data?.find((item) => item.class_id === classId && item.starts_on <= today && (!item.ends_on || item.ends_on >= today))
  const periodAssignments = (assignments.data ?? []).filter((item) => item.academic_period_id === period?.id && (!item.ends_on || item.ends_on >= today))
  const activeTeachers = (teachers.data ?? []).filter((user) => user.is_active && user.teacher_id)
  return { ...directory, period, classes, teachers, activeTeachers, assignments, periodAssignments, classTeachers, activeClassTeacher }
}

/** Строки шаблона текущей четверти по всем классам — одна загрузка для списка классов и списка задач. */
export function usePeriodEntries(periodId: string | undefined) {
  return useQuery({
    queryKey: ['admin-schedule-entries', 'all', periodId],
    queryFn: () => api<ScheduleEntry[]>(`/admin/schedule-entries?academic_period_id=${periodId}`),
    enabled: Boolean(periodId),
  })
}

export function useSchoolAnalytics() {
  return useQuery({ queryKey: ['analytics-school', ''], queryFn: () => api<SchoolAnalytics>('/analytics/school') })
}
