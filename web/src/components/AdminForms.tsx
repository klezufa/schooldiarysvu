import { useEffect, useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import { useToast } from './toastContext'
import { Field } from './UI'

export type FormValues = Record<string, string | number | boolean | Array<string | number> | undefined>
export type FormField = { name: string; label: string; type?: 'text' | 'password' | 'date' | 'number' | 'checkbox' | 'checkboxes' | 'textarea' | 'select'; required?: boolean; options?: { value: string; label: string }[]; hint?: string; min?: number; max?: number; step?: string }

export function AdminForm({ fields, initial, submitLabel = 'Сохранить', onSubmit, onCancel }: {
  fields: FormField[]
  initial?: FormValues
  submitLabel?: string
  onSubmit: (values: FormValues) => Promise<void>
  onCancel?: () => void
}): React.JSX.Element {
  const [values, setValues] = useState<FormValues>(initial ?? {})
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  useEffect(() => { setValues(initial ?? {}); setError(''); setFieldErrors({}) }, [initial])
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setPending(true); setError(''); setFieldErrors({})
    try { await onSubmit(values); if (!onCancel) setValues(initial ?? {}) } catch (caught) {
      if (caught instanceof ApiError) setFieldErrors(caught.fieldErrors)
      setError(caught instanceof Error ? caught.message : 'Не удалось сохранить')
    } finally { setPending(false) }
  }
  return <form className="admin-form" onSubmit={submit}>{fields.map((field) => {
    const value = values[field.name]
    const update = (next: string | number | boolean) => setValues((current) => ({ ...current, [field.name]: next }))
    if (field.type === 'checkboxes') {
      const selected = Array.isArray(value) ? value.map(String) : []
      const toggle = (option: string, checked: boolean) => setValues((current) => ({ ...current, [field.name]: checked ? [...selected, option] : selected.filter((item) => item !== option) }))
      return <fieldset className="checkbox-group" key={field.name}><legend>{field.label}{field.required ? ' *' : ''}</legend>{field.options?.map((option) => <label key={option.value}><input type="checkbox" checked={selected.includes(option.value)} onChange={(event) => toggle(option.value, event.target.checked)} />{option.label}</label>)}{field.hint && <small>{field.hint}</small>}{fieldErrors[field.name] && <small className="field-error" role="alert">{fieldErrors[field.name]}</small>}</fieldset>
    }
    return <Field key={field.name} label={field.label} htmlFor={`admin-${field.name}`} error={fieldErrors[field.name]} hint={field.hint}>{field.type === 'checkbox' ? <input id={`admin-${field.name}`} type="checkbox" checked={Boolean(value)} onChange={(event) => update(event.target.checked)} /> : field.type === 'textarea' ? <textarea id={`admin-${field.name}`} required={field.required} value={String(value ?? '')} onChange={(event) => update(event.target.value)} /> : field.type === 'select' ? <select id={`admin-${field.name}`} required={field.required} value={String(value ?? '')} onChange={(event) => update(event.target.value)}><option value="">Выберите…</option>{field.options?.map((option) => <option value={option.value} key={option.value}>{option.label}</option>)}</select> : <input id={`admin-${field.name}`} type={field.type ?? 'text'} required={field.required} min={field.min} max={field.max} step={field.step} value={String(value ?? '')} onChange={(event) => update(field.type === 'number' ? Number(event.target.value) : event.target.value)} />}</Field>
  })}{error && <div className="form-error" role="alert">{error}</div>}<div className="card-actions"><button className="button primary" disabled={pending}>{pending ? 'Сохраняем…' : submitLabel}</button>{onCancel && <button type="button" className="button secondary" onClick={onCancel}>Отмена</button>}</div></form>
}

export function SaveButton({ onClick, children, danger = false, disabled = false }: { onClick: () => Promise<void>; children: React.ReactNode; danger?: boolean; disabled?: boolean }): React.JSX.Element {
  const [pending, setPending] = useState(false)
  const { showToast } = useToast()
  return <button className={`button ${danger ? 'danger' : 'secondary'}`} disabled={pending || disabled} onClick={async () => { setPending(true); try { await onClick() } catch (error) { showToast(error instanceof Error ? error.message : 'Операция не выполнена', 'error') } finally { setPending(false) } }}>{children}</button>
}

