import {
  Activity, BookOpen, CalendarDays, ClipboardList, FileBarChart, GraduationCap, Home, Megaphone, Settings, ShieldCheck, Users,
} from 'lucide-react'
import type { Role, User } from '../api/types'
import { isAdministrator } from '../api/types'

export type NavigationItem = { to: string; label: string; icon: typeof Home; main?: boolean; section: 'study' | 'manage' | 'account' }

const items: NavigationItem[] = [
  { to: '/today', label: 'Главная', icon: Home, section: 'study' },
  { to: '/journal', label: 'Журнал', icon: BookOpen, section: 'study' },
  { to: '/grades', label: 'Оценки', icon: GraduationCap, section: 'study' },
  { to: '/homework', label: 'Домашние задания', icon: ClipboardList, section: 'study' },
  { to: '/schedule', label: 'Расписание', icon: CalendarDays, section: 'study' },
  { to: '/my-class', label: 'Мой класс', icon: Users, section: 'study' },
  { to: '/reports', label: 'Отчёты', icon: FileBarChart, section: 'study' },
  { to: '/announcements', label: 'Объявления', icon: Megaphone, section: 'study' },
  { to: '/admin', label: 'Администрирование', icon: ShieldCheck, section: 'manage' },
  { to: '/system', label: 'Система', icon: Activity, section: 'manage' },
  { to: '/settings', label: 'Профиль и настройки', icon: Settings, section: 'account' },
]

/** Пункты меню по ролям. Это только удобство интерфейса: права проверяет сервер. */
export function navigationFor(user: User): NavigationItem[] {
  const roles = new Set<Role>(user.roles)
  const staff = roles.has('teacher') || roles.has('class_teacher')
  const learner = roles.has('student') || roles.has('parent')
  const admin = isAdministrator(user.roles)
  const visible = items.filter((item) => {
    switch (item.to) {
      case '/journal': return staff
      case '/grades': return learner
      case '/homework': return staff || learner
      case '/my-class': return roles.has('class_teacher')
      case '/reports': return staff || roles.has('principal')
      case '/admin': return admin
      case '/system': return roles.has('system_admin')
      default: return true
    }
  })
  // В нижнем меню телефона помещаются четыре главных раздела роли.
  const priority = admin && !staff && !learner
    ? ['/today', '/admin', '/schedule', '/system']
    : ['/today', staff ? '/journal' : '/grades', '/schedule', '/homework']
  return visible.map((item) => ({ ...item, main: priority.includes(item.to) }))
}

