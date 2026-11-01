import { useEffect, useId, useState } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link2, Search, TriangleAlert, Unlink } from 'lucide-react'
import { api, apiPage, mutation, queryString } from '../../api/client'
import type { ChildSuggestion, ParentStudentLink, User } from '../../api/types'
import { dateISO } from '../../lib/dates'
import { useToast } from '../../components/toastContext'
import { Modal } from '../../components/UI'
import { useParentLinks } from './parentLinks'

/** Почему ученик предложен: фамилия совпадает или отчество образовано от имени родителя. */
function reasonText(reason: ChildSuggestion['reasons'][number], parentName: string): string {
  if (reason === 'surname') return 'та же фамилия'
  const firstName = parentName.trim().split(/\s+/)[1]
  return firstName ? `отчество от имени «${firstName}»` : 'отчество от имени родителя'
}

/**
 * Связь родителя с детьми. Пока связи нет, родитель не видит ни оценок, ни расписания — поэтому окно сразу
 * предлагает вероятных детей (фамилия, отчество от имени родителя), а любого другого ученика можно найти поиском.
 */
export function ParentLinksDialog({ parent, onClose }: { parent: User | null; onClose: () => void }): React.JSX.Element {
  const id = useId()
  const queryClient = useQueryClient()
  const { showToast } = useToast()
  const { childrenOf, links } = useParentLinks()
  const [search, setSearch] = useState('')
  const [pending, setPending] = useState('')
  useEffect(() => { setSearch('') }, [parent])
  const parentId = parent?.parent_id
  const children = childrenOf(parentId)
  const linkedIds = new Set(children.map((link) => link.student_id))
  const suggestions = useQuery({
    queryKey: ['parent-suggestions', parentId],
    queryFn: () => api<ChildSuggestion[]>(`/parent-student/suggestions?parent_id=${parentId}`),
    enabled: Boolean(parentId),
  })
  const found = useQuery({
    queryKey: ['users', 'link-search', search],
    queryFn: () => apiPage<User>(`/users${queryString({ role: 'student', search: search.trim(), limit: 20 })}`),
    enabled: search.trim().length >= 2,
    placeholderData: keepPreviousData,
  })
  const refresh = async () => {
    for (const key of ['parent-student', 'parent-suggestions', 'analytics-school']) await queryClient.invalidateQueries({ queryKey: [key] })
  }
  const run = async (key: string, action: () => Promise<unknown>, message: string) => {
    setPending(key)
    try {
      await action()
      await refresh()
      showToast(message)
    } catch (caught) {
      showToast(caught instanceof Error ? caught.message : 'Не удалось сохранить', 'error')
    } finally {
      setPending('')
    }
  }
  const link = (studentId: string, name: string) => run(studentId, () => mutation('/parent-student', 'POST', { parent_id: parentId, student_id: studentId, valid_from: dateISO() }),
    `${name}: связь с родителем сохранена`)
  // Удалить связь или завершить её вчерашним днём, решает сервер по школьной дате: часы компьютера могут показывать
  // другие сутки. Доступ пропадает сразу в обоих случаях.
  const unlink = (item: ParentStudentLink) => {
    if (!window.confirm(`Отвязать ${item.student_name}? Родитель перестанет видеть оценки и расписание этого ребёнка.`)) return
    void run(item.student_id, () => mutation(`/parent-student${queryString({ parent_id: item.parent_id, student_id: item.student_id, valid_from: item.valid_from })}`, 'DELETE'),
      `${item.student_name}: связь снята`)
  }
  const searchResults = (found.data?.items ?? []).filter((user) => user.student_id && !linkedIds.has(user.student_id))
  return (
    <Modal open={Boolean(parent)} onOpenChange={(open) => { if (!open) onClose() }} title={`Дети: ${parent?.display_name ?? ''}`}
      description="Родитель видит оценки, расписание и задания только тех детей, с которыми связан." wide>
      {parent && (
        <div className="stack-form parent-links">
          {!links.isLoading && children.length === 0 && (
            <div className="notice" role="status"><TriangleAlert aria-hidden="true" />
              <span><strong>Родитель не связан с ребёнком.</strong> Пока связи нет, он не видит оценки, расписание и задания. Выберите ребёнка ниже — всё появится у родителя сразу.</span>
            </div>
          )}
          {children.length > 0 && (
            <section>
              <h3>Связан с</h3>
              <ul className="simple-list compact">
                {children.map((item) => (
                  <li key={item.student_id}>
                    <div><strong>{item.student_name}</strong><small>{item.class_name ? `${item.class_name} класс` : 'без класса'}</small></div>
                    <button type="button" className="button secondary small" disabled={pending === item.student_id} onClick={() => unlink(item)}><Unlink aria-hidden="true" />Отвязать</button>
                  </li>
                ))}
              </ul>
            </section>
          )}
          <section>
            <h3>Похожи на детей этого родителя</h3>
            {suggestions.isLoading ? <p className="muted">Ищем…</p> : suggestions.data?.length ? (
              <ul className="simple-list compact" aria-label="Подсказки">
                {suggestions.data.map((item) => (
                  <li key={item.student_id}>
                    <div>
                      <strong>{item.display_name}</strong>
                      <small>{item.class_name ? `${item.class_name} класс` : 'без класса'}</small>
                      <span className="reason-chips">{item.reasons.map((reason) => <span key={reason} className="badge badge-info">{reasonText(reason, parent.display_name)}</span>)}</span>
                    </div>
                    <button type="button" className="button primary small" disabled={pending === item.student_id} onClick={() => void link(item.student_id, item.display_name)}>
                      <Link2 aria-hidden="true" />Связать
                    </button>
                  </li>
                ))}
              </ul>
            ) : <p className="muted">Совпадений по фамилии и отчеству не нашлось — найдите ученика поиском.</p>}
          </section>
          <section>
            <h3><label htmlFor={`${id}-search`}>Найти ученика</label></h3>
            <span className="input-action"><Search aria-hidden="true" /><input id={`${id}-search`} value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Фамилия ученика, от двух букв" /></span>
            {search.trim().length >= 2 && (
              searchResults.length ? (
                <ul className="simple-list compact">
                  {searchResults.map((user) => (
                    <li key={user.id}>
                      <div><strong>{user.display_name}</strong><small>{user.class_name ? `${user.class_name} класс` : 'без класса'}</small></div>
                      <button type="button" className="button secondary small" disabled={pending === user.student_id} onClick={() => void link(user.student_id!, user.display_name)}>
                        <Link2 aria-hidden="true" />Связать
                      </button>
                    </li>
                  ))}
                </ul>
              ) : !found.isLoading && <p className="muted">Никого не нашли.</p>
            )}
          </section>
          <div className="dialog-actions"><button type="button" className="button primary" onClick={onClose}>Готово</button></div>
        </div>
      )}
    </Modal>
  )
}
