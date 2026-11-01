import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { ParentStudentLink } from '../../api/types'
import { dateISO } from '../../lib/dates'

/** Связь действует сегодня: началась и не завершена. */
export function isActiveLink(link: Pick<ParentStudentLink, 'valid_from' | 'valid_to'>, today = dateISO()): boolean {
  return link.valid_from <= today && (!link.valid_to || link.valid_to >= today)
}

/** Связи родителей и детей с именами; childrenOf — действующие связи одного родителя. */
export function useParentLinks() {
  const links = useQuery({ queryKey: ['parent-student'], queryFn: () => api<ParentStudentLink[]>('/parent-student') })
  const today = dateISO()
  const active = (links.data ?? []).filter((link) => isActiveLink(link, today))
  return {
    links,
    childrenOf: (parentId?: string) => active.filter((link) => link.parent_id === parentId),
    hasChildren: (parentId?: string) => active.some((link) => link.parent_id === parentId),
  }
}
