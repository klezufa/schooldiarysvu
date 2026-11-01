import { ArrowLeft, TriangleAlert } from 'lucide-react'
import { Modal } from '../../../components/UI'
import { CopyButton } from '../shared'
import type { Credential } from './actions'

export function BackToTasks({ onBack, label = 'К списку задач' }: { onBack: () => void; label?: string }): React.JSX.Element {
  return <button type="button" className="button text-button simple-back" onClick={onBack}><ArrowLeft aria-hidden="true" />{label}</button>
}

/** Логины и временные пароли созданных людей: показываются один раз, их нужно сразу передать. */
export function CredentialsDialog({ credentials, onClose }: { credentials: Credential[] | null; onClose: () => void }): React.JSX.Element {
  const text = (credentials ?? []).map((item) => `${item.role}: ${item.name}\nЛогин: ${item.login}\nВременный пароль: ${item.password}`).join('\n\n')
  return (
    <Modal open={Boolean(credentials)} onOpenChange={(open) => { if (!open) onClose() }} title="Данные для входа"
      description="Передайте каждому его логин и временный пароль. При первом входе дневник попросит придумать свой пароль." wide>
      {credentials && (
        <div className="stack-form">
          <ul className="credential-list">
            {credentials.map((item) => (
              <li key={item.login}>
                <span className="muted">{item.role}</span>
                <strong>{item.name}</strong>
                <span>Логин: <code data-testid="credential-login">{item.login}</code></span>
                <span>Пароль: <code data-testid="credential-password">{item.password}</code></span>
              </li>
            ))}
          </ul>
          <div className="notice"><TriangleAlert aria-hidden="true" /><span>После закрытия окна пароли больше не будут показаны. Если пароль потеряют — выдайте новый в разделе «Пароли и вход».</span></div>
          <div className="dialog-actions">
            <CopyButton text={text} label="Скопировать всё" />
            <button type="button" className="button primary" onClick={onClose}>Готово</button>
          </div>
        </div>
      )}
    </Modal>
  )
}
