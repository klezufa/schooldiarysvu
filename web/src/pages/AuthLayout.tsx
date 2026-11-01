import type { ReactNode } from 'react'
import { ShieldCheck } from 'lucide-react'
import { BookMark, SchoolIllustration } from '../components/Illustrations'
import { OfflineBanner } from '../components/UI'

/** Общая раскладка входа и регистрации: слева бренд и иллюстрация, справа карточка формы. */
export function AuthLayout({ children }: { children: ReactNode }): React.JSX.Element {
  return (
    <main className="auth-page">
      <OfflineBanner />
      <section className="auth-hero" aria-label="О системе">
        <div className="auth-brand"><span className="brand-mark" aria-hidden="true"><BookMark /></span>Школьный дневник</div>
        <div>
          <h1>Учёба. Развитие. Успех.</h1>
          <p>Расписание, оценки, задания и объявления школы — в одном месте, для учеников, родителей и учителей.</p>
        </div>
        <div className="auth-art" aria-hidden="true"><SchoolIllustration /></div>
        <div className="auth-safe">
          <ShieldCheck aria-hidden="true" />
          <div><strong>Ваши данные в безопасности</strong>Система работает в локальной сети школы и не передаёт данные внешним сервисам.</div>
        </div>
      </section>
      <section className="auth-panel">{children}</section>
    </main>
  )
}
