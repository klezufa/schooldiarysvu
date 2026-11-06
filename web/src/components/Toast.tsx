import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { AnimatePresence } from 'motion/react'
import * as m from 'motion/react-m'
import { CheckCircle2, CircleAlert, X } from 'lucide-react'
import { ToastContext, type ToastItem, type ToastKind } from './toastContext'

export function ToastProvider({ children }: { children: ReactNode }): React.JSX.Element {
  const [items, setItems] = useState<ToastItem[]>([])
  const dismiss = useCallback((id: number) => setItems((current) => current.filter((item) => item.id !== id)), [])
  const showToast = useCallback((text: string, kind: ToastKind = 'success') => {
    const id = Date.now() + Math.random()
    setItems((current) => [...current.slice(-2), { id, kind, text }])
    window.setTimeout(() => dismiss(id), 5000)
  }, [dismiss])
  const value = useMemo(() => ({ showToast }), [showToast])
  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-region" aria-live="polite" aria-atomic="false">
        <AnimatePresence initial={false}>
          {items.map((item) => (
            <m.div
              layout
              className={`toast toast-${item.kind}`}
              key={item.id}
              role={item.kind === 'error' ? 'alert' : 'status'}
              initial={{ opacity: 0, y: 14, scale: .97 }}
              animate={{ opacity: 1, y: 0, scale: 1, transition: { type: 'spring', stiffness: 420, damping: 32 } }}
              exit={{ opacity: 0, scale: .96, transition: { duration: .14 } }}
            >
              {item.kind === 'success' ? <CheckCircle2 aria-hidden="true" /> : <CircleAlert aria-hidden="true" />}
              <span>{item.text}</span>
              <button className="icon-button" onClick={() => dismiss(item.id)} aria-label="Закрыть уведомление"><X /></button>
            </m.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  )
}

