import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link2, Search } from 'lucide-react'
import { apiPage, queryString } from '../../../api/client'
import type { User } from '../../../api/types'
import { QueryState, StatusBadge } from '../../../components/UI'
import { ParentLinksDialog } from '../ParentLinksDialog'
import { useParentLinks } from '../parentLinks'
import { BackToTasks } from './common'

/** Родители и их дети. Родитель без связи не видит ничего — такие показываются первыми и с пометкой. */
export function ParentsView({ onBack }: { onBack: () => void }): React.JSX.Element {
  const [search, setSearch] = useState('')
  const [onlyUnlinked, setOnlyUnlinked] = useState(true)
  const [linking, setLinking] = useState<User | null>(null)
  const { childrenOf, hasChildren, links } = useParentLinks()
  const parents = useQuery({
    queryKey: ['users', 'simple-parents', search],
    queryFn: () => apiPage<User>(`/users${queryString({ role: 'parent', search: search.trim() || undefined, limit: 100 })}`),
    placeholderData: keepPreviousData,
  })
  const all = (parents.data?.items ?? []).filter((user) => user.parent_id)
  const unlinked = all.filter((user) => !hasChildren(user.parent_id))
  const shown = (onlyUnlinked ? unlinked : [...unlinked, ...all.filter((user) => hasChildren(user.parent_id))])
  return (
    <section className="panel">
      <BackToTasks onBack={onBack} />
      <div className="section-heading">
        <div><h2>Родители и дети</h2><p className="muted">Родитель видит оценки, расписание и задания только тех детей, с которыми связан. Нажмите «Связать с ребёнком» — дневник подскажет подходящих учеников по фамилии и отчеству.</p></div>
      </div>
      <div className="filter-bar">
        <label className="grow">Поиск<span className="input-action"><Search aria-hidden="true" /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Фамилия родителя" /></span></label>
        <label className="checkbox-label"><input type="checkbox" checked={onlyUnlinked} onChange={(event) => setOnlyUnlinked(event.target.checked)} />Только без ребёнка ({unlinked.length})</label>
      </div>
      <QueryState isLoading={parents.isLoading || links.isLoading} error={parents.error ?? links.error} retry={parents.refetch} isEmpty={!shown.length}
        emptyTitle={onlyUnlinked && all.length ? 'Все родители связаны с детьми' : 'Родителей не найдено'}
        emptyText={onlyUnlinked && all.length ? 'Снимите отметку «Только без ребёнка», чтобы увидеть всех.' : 'Родители появляются при добавлении ученика или импорте списка.'}>
        <ul className="simple-list">
          {shown.map((user) => {
            const children = childrenOf(user.parent_id)
            return (
              <li key={user.id}>
                <div>
                  <strong>{user.display_name}</strong>
                  {children.length
                    ? <small>Дети: {children.map((link) => `${link.student_name}${link.class_name ? ` (${link.class_name})` : ''}`).join(', ')}</small>
                    : <small><StatusBadge tone="warning">Не связан с ребёнком</StatusBadge> пока не видит оценки</small>}
                </div>
                <button type="button" className={`button small ${children.length ? 'secondary' : 'primary'}`} onClick={() => setLinking(user)}>
                  <Link2 aria-hidden="true" />{children.length ? 'Дети' : 'Связать с ребёнком'}
                </button>
              </li>
            )
          })}
        </ul>
      </QueryState>
      <ParentLinksDialog parent={linking} onClose={() => setLinking(null)} />
    </section>
  )
}
