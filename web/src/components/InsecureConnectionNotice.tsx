import { ShieldAlert } from 'lucide-react'

/**
 * Профиль local-http: пароли и данные идут по сети открытым текстом. Предупреждение видит администратор,
 * когда дневник открыт по HTTP не с этого же компьютера (localhost — безопасный локальный случай).
 */
export function InsecureConnectionNotice(): React.JSX.Element | null {
  if (typeof window === 'undefined') return null
  const { protocol, hostname } = window.location
  const local = hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '::1' || hostname === '[::1]'
  if (protocol !== 'http:' || local) return null
  return (
    <div className="notice insecure-notice" role="alert">
      <ShieldAlert aria-hidden="true" />
      <span>
        <strong>Соединение не зашифровано (профиль local-http).</strong> Пароли и оценки передаются по сети открытым текстом —
        подходит только для изолированной сети или пилота. Для постоянной работы включите профиль https (см. docs/https.md).
      </span>
    </div>
  )
}
