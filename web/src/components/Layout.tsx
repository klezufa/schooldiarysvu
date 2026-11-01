import { useState } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, Link2Off, LogOut, Menu, ShieldHalf, X } from 'lucide-react'
import { api, mutation, queryString } from '../api/client'
import type { Notification } from '../api/types'
import { roleNames } from '../api/types'
import { navigationFor, type NavigationItem } from '../lib/navigation'
import { useAuth } from '../authContext'
import { useStudent } from '../studentContext'
import { dateISO, formatDate } from '../lib/dates'
import { BookMark } from './Illustrations'
import { Modal, OfflineBanner, QueryState, StatusBadge } from './UI'

const sectionTitles: Record<NavigationItem['section'], string> = { study: 'Учёба', manage: 'Управление', account: 'Профиль' }

function Navigation({ onNavigate }: { onNavigate?: () => void }): React.JSX.Element {
  const { user } = useAuth()
  if (!user) return <></>
  const navigation = navigationFor(user)
  const sections = (['study', 'manage', 'account'] as const).filter((section) => navigation.some((item) => item.section === section))
  return (
    <nav className="side-nav" aria-label="Основная навигация">
      {sections.map((section) => (
        <div key={section} className="side-nav-group">
          {section !== 'study' && <div className="nav-section">{sectionTitles[section]}</div>}
          {navigation.filter((item) => item.section === section).map(({ to, label, icon: Icon }) => (
            <NavLink key={to} to={to} onClick={onNavigate} className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
              <Icon aria-hidden="true" />
              <span>{label}</span>
            </NavLink>
          ))}
        </div>
      ))}
    </nav>
  )
}

function Notifications({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }): React.JSX.Element {
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey: ['notifications'], queryFn: () => api<Notification[]>(`/notifications${queryString({ limit: 50 })}`), enabled: open })
  const read = useMutation({
    mutationFn: (id: string) => mutation<void>(`/notifications/${id}/read`, 'POST'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notifications'] }),
  })
  return (
    <Modal open={open} onOpenChange={onOpenChange} title="Уведомления" description="Внутренние сообщения системы">
      <QueryState isLoading={query.isLoading} error={query.error} retry={query.refetch} isEmpty={!query.data?.length} emptyTitle="Новых уведомлений нет">
        <div className="notification-list">
          {query.data?.map((item) => (
            <article className={item.read_at ? 'notification read' : 'notification'} key={item.id}>
              <div>
                <StatusBadge tone={item.read_at ? 'neutral' : 'info'}>{item.read_at ? 'Прочитано' : 'Новое'}</StatusBadge>
                <time>{formatDate(item.created_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}</time>
              </div>
              <h3>{item.title}</h3>
              <p>{item.body}</p>
              {!item.read_at && <button className="button text-button" disabled={read.isPending} onClick={() => read.mutate(item.id)}>Отметить прочитанным</button>}
            </article>
          ))}
        </div>
      </QueryState>
    </Modal>
  )
}

const pageTitles: Record<string, string> = {
  '/today': 'Главная', '/journal': 'Электронный журнал', '/grades': 'Успеваемость', '/homework': 'Домашние задания',
  '/schedule': 'Расписание', '/my-class': 'Мой класс', '/reports': 'Отчёты', '/announcements': 'Объявления',
  '/admin': 'Администрирование', '/system': 'Состояние системы', '/settings': 'Профиль и настройки',
}

function UnlinkedParentNotice({ onlyParent }: { onlyParent: boolean }): React.JSX.Element {
  return (
    <section className={onlyParent ? 'panel unlinked-parent' : 'notice unlinked-parent-banner'} role="status">
      <Link2Off aria-hidden="true" />
      <div>
        <h2>Вы пока не связаны с ребёнком</h2>
        <p>Оценки, расписание и задания появятся здесь, как только администратор школы свяжет вашу учётную запись с учётной записью ребёнка.
          Обратитесь к классному руководителю или администратору и назовите фамилию, имя и класс ребёнка.</p>
      </div>
    </section>
  )
}

export function Layout(): React.JSX.Element {
  const { user, logout } = useAuth()
  const { children, studentId, setStudentId, noChildren } = useStudent()
  const [moreOpen, setMoreOpen] = useState(false)
  const [notificationsOpen, setNotificationsOpen] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const unread = useQuery({ queryKey: ['notifications'], queryFn: () => api<Notification[]>('/notifications?limit=50'), refetchInterval: 60_000 })
  if (!user) return <></>
  const navigation = navigationFor(user)
  const primary = navigation.filter((item) => item.main).slice(0, 4)
  const unreadCount = unread.data?.filter((item) => !item.read_at).length ?? 0
  const signOut = async () => { await logout(); navigate('/login', { replace: true }) }
  const roleLine = user.roles.map((role) => roleNames[role]).join(' · ')
  return (
    <div className="app-layout">
      <OfflineBanner />
      <aside className="sidebar">
        <NavLink to="/today" className="brand">
          <span className="brand-mark" aria-hidden="true"><BookMark /></span>
          <span>Школьный<br />дневник</span>
        </NavLink>
        <Navigation />
        <div className="sidebar-foot">
          <ShieldHalf aria-hidden="true" />
          <div><strong>Данные в безопасности</strong>Система работает в локальной сети школы</div>
        </div>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <div className="topbar-title">
            <strong>{pageTitles[location.pathname] ?? 'Школьный дневник'}</strong>
            <span>{formatDate(dateISO(), { weekday: 'long', day: 'numeric', month: 'long' })}</span>
          </div>
          {user.roles.includes('parent') && !noChildren && (
            <div className="child-switcher">
              <label htmlFor="child-select">Ребёнок</label>
              <select
                id="child-select"
                value={studentId ?? ''}
                onChange={(event) => {
                  setStudentId(event.target.value)
                  queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'me' && query.queryKey[0] !== 'children' })
                }}
              >
                {children.map((child) => <option value={child.id} key={child.id}>{child.display_name}</option>)}
              </select>
            </div>
          )}
          <button className="icon-button notification-button" onClick={() => setNotificationsOpen(true)} aria-label={`Уведомления, непрочитанных: ${unreadCount}`}>
            <Bell />{unreadCount > 0 && <span>{unreadCount}</span>}
          </button>
          <div className="user-summary">
            <span className="avatar" aria-hidden="true">{user.display_name.slice(0, 1)}</span>
            <div><strong>{user.display_name}</strong><span>{roleLine}</span></div>
          </div>
          <button className="icon-button logout-button" onClick={signOut} aria-label="Выйти" title="Выйти"><LogOut aria-hidden="true" /></button>
        </header>
        <main className="main-content" id="main-content">
          {/* Родителю без связи с ребёнком нечего показать — объясняем почему на всех страницах, кроме настроек профиля. */}
          {noChildren && location.pathname !== '/settings' ? <UnlinkedParentNotice onlyParent={user.roles.every((role) => role === 'parent')} /> : null}
          {!noChildren || location.pathname === '/settings' || !user.roles.every((role) => role === 'parent') ? <Outlet /> : null}
        </main>
      </div>
      <nav className="mobile-nav" aria-label="Мобильная навигация">
        {primary.map(({ to, label, icon: Icon }) => (
          <NavLink key={to} to={to} className={({ isActive }) => (isActive ? 'active' : '')}><Icon aria-hidden="true" /><span>{label}</span></NavLink>
        ))}
        <button onClick={() => setMoreOpen(true)} className={moreOpen ? 'active' : ''}><Menu aria-hidden="true" /><span>Ещё</span></button>
      </nav>
      {moreOpen && (
        <div className="mobile-menu-overlay" onClick={() => setMoreOpen(false)}>
          <section className="mobile-menu" role="dialog" aria-modal="true" aria-label="Все разделы" onClick={(event) => event.stopPropagation()}>
            <div className="dialog-heading">
              <h2>Разделы</h2>
              <button className="icon-button" autoFocus onClick={() => setMoreOpen(false)} aria-label="Закрыть меню"><X /></button>
            </div>
            <Navigation onNavigate={() => setMoreOpen(false)} />
            <button className="button secondary" onClick={signOut}><LogOut />Выйти</button>
          </section>
        </div>
      )}
      <Notifications open={notificationsOpen} onOpenChange={setNotificationsOpen} />
    </div>
  )
}
