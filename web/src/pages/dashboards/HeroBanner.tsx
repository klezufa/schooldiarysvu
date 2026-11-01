import type { ReactNode } from 'react'
import { DeskIllustration } from '../../components/Illustrations'

export function HeroBanner({ title, text, meta, art = true }: { title: string; text: string; meta?: ReactNode; art?: boolean }): React.JSX.Element {
  return (
    <section className="hero-banner">
      <div>
        <h1>{title}</h1>
        <p>{text}</p>
        {meta && <div className="hero-meta">{meta}</div>}
      </div>
      {art && <div className="hero-art" aria-hidden="true"><DeskIllustration /></div>}
    </section>
  )
}

