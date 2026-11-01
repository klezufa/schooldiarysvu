import { isAdministrator } from '../api/types'
import { useAuth } from '../authContext'
import { SchoolDashboard } from './dashboards/SchoolDashboard'
import { StudentDashboard } from './dashboards/StudentDashboard'
import { TeacherDashboard } from './dashboards/TeacherDashboard'

/** Главная страница: дашборд по основной роли пользователя. */
export function TodayPage(): React.JSX.Element {
  const { hasRole, user } = useAuth()
  if (hasRole('teacher', 'class_teacher')) return <TeacherDashboard />
  if (hasRole('student', 'parent')) return <StudentDashboard />
  if (user && (isAdministrator(user.roles) || hasRole('principal'))) return <SchoolDashboard />
  return <StudentDashboard />
}
