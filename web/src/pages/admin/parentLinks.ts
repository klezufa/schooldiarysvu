import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { ParentStudentLink } from '../../api/types'

/**
 * Связи родителей и детей с именами; childrenOf — действующие связи одного родителя. Действует ли связь сегодня,
 * решает сервер по часовому поясу школы: у компьютера администратора могут быть другие сутки.
 */
export function useParentLinks() {
  const links = useQuery({ queryKey: ['parent-student'], queryFn: () => api<ParentStudentLink[]>('/parent-student') })
  const active = (links.data ?? []).filter((link) => link.active)
  return {
    links,
    childrenOf: (parentId?: string) => active.filter((link) => link.parent_id === parentId),
    hasChildren: (parentId?: string) => active.some((link) => link.parent_id === parentId),
  }
}
