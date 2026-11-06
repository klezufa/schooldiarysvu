import { Component, useEffect, useState, type ErrorInfo, type ReactNode } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { AlertTriangle, CloudOff, LockKeyhole, RefreshCw, X } from 'lucide-react'
import { ApiError } from '../api/client'

export function Loading({ label = 'Загружаем данные…' }: { label?: string }): React.JSX.Element {
  return <div className="state-view" role="status"><span className="spinner" aria-hidden="true" /><p>{label}</p></div>
}

/** Скелетон списка: повторяет форму строк, пока данные загружаются, чтобы страница не прыгала. */
export function Skeleton({ rows = 3, label = 'Загружаем данные…' }: { rows?: number; label?: string }): React.JSX.Element {
  return (
    <div className="skeleton-stack" role="status" aria-live="polite">
      <span className="sr-only">{label}</span>
      {Array.from({ length: rows }, (_, index) => (
        <div className="skeleton-row" key={index} aria-hidden="true">
          <span className="skeleton circle" />
          <div><span className={`skeleton ${index % 2 ? 'w-60' : 'w-80'}`} /><span className="skeleton w-40" /></div>
        </div>
      ))}
    </div>
  )
}

export function Empty({ title, text, action }: { title: string; text?: string; action?: ReactNode }): React.JSX.Element {
  return <div className="state-view empty-state"><h3>{title}</h3>{text && <p>{text}</p>}{action}</div>
}

export function StateView({ error, retry }: { error: unknown; retry?: () => void }): React.JSX.Element {
  const apiError = error instanceof ApiError ? error : undefined
  const offline = apiError?.status === 0
  const forbidden = apiError?.status === 403
  const conflict = apiError?.status === 409
  const title = offline ? 'Нет соединения' : forbidden ? 'Доступ ограничен' : conflict ? 'Конфликт данных' : apiError?.status === 429 ? 'Слишком много запросов' : 'Не удалось загрузить данные'
  const Icon = offline ? CloudOff : forbidden ? LockKeyhole : AlertTriangle
  return (
    <div className={`state-view ${conflict ? 'conflict-state' : 'error-state'}`} role="alert">
      <Icon aria-hidden="true" />
      <h3>{title}</h3>
      <p>{error instanceof Error ? error.message : 'Произошла неизвестная ошибка.'}</p>
      {apiError?.requestId && <small>Код запроса: {apiError.requestId}</small>}
      {retry && <button className="button secondary" onClick={retry}><RefreshCw aria-hidden="true" />Повторить</button>}
    </div>
  )
}

export function QueryState({ isLoading, error, retry, isEmpty, emptyTitle, emptyText, children }: {
  isLoading: boolean; error: unknown; retry?: () => void; isEmpty?: boolean; emptyTitle?: string; emptyText?: string; children: ReactNode
}): React.JSX.Element {
  if (isLoading) return <Skeleton />
  if (error) return <StateView error={error} retry={retry} />
  if (isEmpty) return <Empty title={emptyTitle ?? 'Данных пока нет'} text={emptyText} />
  return <>{children}</>
}

export function Modal({ open, onOpenChange, title, description, children, wide = false, onOpenAutoFocus }: {
  open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: string; children: ReactNode; wide?: boolean; onOpenAutoFocus?: (event: Event) => void
}): React.JSX.Element {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className={`dialog-content ${wide ? 'dialog-wide' : ''}`} onOpenAutoFocus={onOpenAutoFocus}>
          <div className="dialog-heading">
            <div><Dialog.Title>{title}</Dialog.Title>{description && <Dialog.Description>{description}</Dialog.Description>}</div>
            <Dialog.Close className="icon-button" aria-label="Закрыть"><X /></Dialog.Close>
          </div>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

export function Field({ label, error, hint, children, htmlFor }: {
  label: string; error?: string; hint?: string; children: ReactNode; htmlFor?: string
}): React.JSX.Element {
  return <div className={`field ${error ? 'field-invalid' : ''}`}><label htmlFor={htmlFor}>{label}</label>{children}{hint && !error && <small>{hint}</small>}{error && <small className="field-error" role="alert">{error}</small>}</div>
}

export function PageHeader({ eyebrow, title, description, actions }: {
  eyebrow?: string; title: string; description?: string; actions?: ReactNode
}): React.JSX.Element {
  return <header className="page-header"><div>{eyebrow && <p className="eyebrow">{eyebrow}</p>}<h1>{title}</h1>{description && <p>{description}</p>}</div>{actions && <div className="page-actions">{actions}</div>}</header>
}

export function StatusBadge({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'info' }): React.JSX.Element {
  return <span className={`badge badge-${tone}`}>{children}</span>
}

export function OfflineBanner(): React.JSX.Element | null {
  const [online, setOnline] = useState(() => navigator.onLine)
  useEffect(() => {
    const update = () => setOnline(navigator.onLine)
    window.addEventListener('online', update)
    window.addEventListener('offline', update)
    return () => { window.removeEventListener('online', update); window.removeEventListener('offline', update) }
  }, [])
  if (online) return null
  return <div className="offline-banner" role="status"><CloudOff aria-hidden="true" />Нет соединения. Изменения не отправляются; не закрывайте страницу с несохранённым вводом.</div>
}

export class ErrorBoundary extends Component<{ children: ReactNode }, { error?: Error }> {
  state: { error?: Error } = {}
  static getDerivedStateFromError(error: Error): { error: Error } { return { error } }
  componentDidCatch(error: Error, info: ErrorInfo): void { console.error('Page rendering failed', error, info.componentStack) }
  render(): ReactNode {
    if (this.state.error) return <main className="shell"><StateView error={this.state.error} retry={() => this.setState({ error: undefined })} /></main>
    return this.props.children
  }
}
