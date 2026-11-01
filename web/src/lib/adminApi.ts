import { mutation } from '../api/client'
import type { FormValues } from '../components/AdminForms'

export async function send(path: string, method: 'POST' | 'PUT' | 'PATCH' | 'DELETE', values?: FormValues): Promise<void> {
  await mutation(path, method, values)
}
