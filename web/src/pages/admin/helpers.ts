import { useQuery } from '@tanstack/react-query'
import { api, apiPage } from '../../api/client'
import type { AcademicPeriod, AcademicYear, NamedEntity, Role, User } from '../../api/types'
import { useAuth } from '../../authContext'
import { classLabeler, classesOfYear, openClasses, pickCurrentPeriod } from '../../lib/academicYears'

export function useIsSystemAdmin(): boolean {
  const { hasRole } = useAuth()
  return hasRole('system_admin')
}

/**
 * Справочники админки с общими ключами кэша — после изменения достаточно инвалидировать ключ. Периоды — всех неархивных
 * лет: следующий год администратор готовит заранее, до того как системный администратор сделает его текущим.
 */
export function useDirectory() {
  const classes = useQuery({ queryKey: ['classes'], queryFn: () => api<NamedEntity[]>('/classes') })
  const subjects = useQuery({ queryKey: ['subjects'], queryFn: () => api<NamedEntity[]>('/subjects') })
  const rooms = useQuery({ queryKey: ['rooms'], queryFn: () => api<NamedEntity[]>('/rooms') })
  const years = useQuery({ queryKey: ['academic-years'], queryFn: () => api<AcademicYear[]>('/academic-years') })
  const year = years.data?.find((item) => item.is_current) ?? years.data?.[0]
  const periods = useQuery({ queryKey: ['academic-periods', 'all'], queryFn: () => api<AcademicPeriod[]>('/academic-periods') })
  const currentPeriod = pickCurrentPeriod(periods.data?.filter((period) => !year || period.academic_year_id === year.id))
    ?? pickCurrentPeriod(periods.data)
  const classLabel = classLabeler(classes.data, years.data)
  const yearName = (id?: string) => years.data?.find((item) => item.id === id)?.name ?? ''
  /** Варианты для выпадающего списка классов с подписью года, если в списке классы разных лет. */
  const classOptions = (list: NamedEntity[] | undefined = classes.data) => {
    const label = classLabeler(list, years.data)
    return (list ?? []).map((item) => ({ value: item.id, label: label(item) }))
  }
  return {
    classes, subjects, rooms, years, year, periods, currentPeriod, classLabel, classOptions, yearName,
    openClasses: openClasses(classes.data, years.data),
    classesOfYear: (yearId?: string) => classesOfYear(classes.data, yearId),
  }
}

/** Люди по роли (до 100) — для выпадающих списков назначений. */
export function usePeople(role: Role) {
  return useQuery({
    queryKey: ['users', 'by-role', role],
    queryFn: async () => (await apiPage<User>(`/users?role=${role}&limit=100`)).items,
  })
}

export const options = (items?: { id: string; name?: string; display_name?: string }[]) =>
  items?.map((item) => ({ value: item.id, label: item.name ?? item.display_name ?? item.id })) ?? []

/** Временный пароль на стороне браузера (для формы создания), тем же алфавитом без похожих символов. */
export function randomPassword(): string {
  const alphabet = 'ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789'
  const bytes = new Uint32Array(14)
  crypto.getRandomValues(bytes)
  const chars = Array.from(bytes, (value) => alphabet[value % alphabet.length])
  return `${chars.slice(0, 4).join('')}-${chars.slice(4, 9).join('')}-${chars.slice(9).join('')}`
}

const translit: Record<string, string> = {
  а: 'a', б: 'b', в: 'v', г: 'g', д: 'd', е: 'e', ё: 'e', ж: 'zh', з: 'z', и: 'i', й: 'y', к: 'k', л: 'l', м: 'm', н: 'n', о: 'o',
  п: 'p', р: 'r', с: 's', т: 't', у: 'u', ф: 'f', х: 'kh', ц: 'ts', ч: 'ch', ш: 'sh', щ: 'shch', ъ: '', ы: 'y', ь: '', э: 'e', ю: 'yu', я: 'ya',
}

/** Подсказка логина из ФИО — та же схема, что на сервере при импорте: «Иванов Иван Петрович» → ivanov.ip. */
export function suggestLogin(fullName: string): string {
  const latin = (word: string) => [...word.toLowerCase()].map((char) => translit[char] ?? (/[a-z0-9]/.test(char) ? char : '')).join('')
  const [surname, ...rest] = fullName.trim().split(/\s+/)
  if (!surname) return ''
  const initials = rest.map((part) => latin(part).slice(0, 1)).join('')
  return `${latin(surname)}${initials ? `.${initials}` : ''}`.slice(0, 56)
}
