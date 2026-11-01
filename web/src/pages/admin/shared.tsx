import { useState } from 'react'
import { Check, Copy, TriangleAlert } from 'lucide-react'
import type { AcademicPeriod, AcademicYear, Role } from '../../api/types'
import { roleNames } from '../../api/types'
import { Modal } from '../../components/UI'

export function RolePills({ roles }: { roles: Role[] }): React.JSX.Element {
  return <span className="role-pills">{roles.map((role) => <span key={role} className={`role-pill role-${role}`}>{roleNames[role]}</span>)}</span>
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

export function CopyButton({ text, label = 'Копировать' }: { text: string; label?: string }): React.JSX.Element {
  const [copied, setCopied] = useState(false)
  return (
    <button type="button" className="button secondary small" onClick={async () => { setCopied(await copyText(text)) }}>
      {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}{copied ? 'Скопировано' : label}
    </button>
  )
}

/** Одноразовый секрет (временный пароль, код приглашения): показывается один раз, сервер его не хранит. */
export function SecretDialog({ secret, onClose }: {
  secret: { title: string; value: string; text: string; extra?: { label: string; value: string } } | null
  onClose: () => void
}): React.JSX.Element {
  return (
    <Modal open={Boolean(secret)} onOpenChange={(open) => { if (!open) onClose() }} title={secret?.title ?? ''} description={secret?.text}>
      {secret && (
        <div className="stack-form">
          <div className="secret-box"><code data-testid="secret-value">{secret.value}</code><CopyButton text={secret.value} /></div>
          {secret.extra && (
            <div className="field">
              <label>{secret.extra.label}</label>
              <div className="secret-box"><code style={{ fontSize: '.85rem' }}>{secret.extra.value}</code><CopyButton text={secret.extra.value} /></div>
            </div>
          )}
          <div className="notice"><TriangleAlert aria-hidden="true" /><span>Запишите или передайте сейчас: после закрытия окна значение больше не будет показано. Если его потеряют — выпустите новое.</span></div>
          <div className="dialog-actions"><button className="button primary" onClick={onClose}>Готово</button></div>
        </div>
      )}
    </Modal>
  )
}


/**
 * Выбор учебного периода, сгруппированный по учебным годам: названия четвертей в разных годах совпадают,
 * а следующий год администратор готовит заранее.
 */
export function PeriodSelect({ id, value, onChange, periods, years }: {
  id?: string; value: string; onChange: (period: AcademicPeriod) => void; periods: AcademicPeriod[] | undefined; years: AcademicYear[] | undefined
}): React.JSX.Element {
  const groups = [...(years ?? [])]
    .sort((a, b) => a.starts_on.localeCompare(b.starts_on))
    .map((year) => ({ year, items: (periods ?? []).filter((period) => period.academic_year_id === year.id) }))
    .filter((group) => group.items.length)
  const option = (period: AcademicPeriod) => <option key={period.id} value={period.id}>{period.name}{period.status === 'closed' ? ' (закрыт)' : ''}</option>
  return (
    <select id={id} value={value} onChange={(event) => { const next = periods?.find((period) => period.id === event.target.value); if (next) onChange(next) }}>
      {groups.length > 1
        ? groups.map((group) => <optgroup key={group.year.id} label={`${group.year.name} учебный год`}>{group.items.map(option)}</optgroup>)
        : (periods ?? []).map(option)}
    </select>
  )
}
