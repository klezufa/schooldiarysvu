import type { OperationsOverview } from '../api/types'
import { operationChecks } from './operations'

const now = new Date('2026-11-10T12:00:00Z')
const base: OperationsOverview = { network_profile: 'https', tls_names: [] }
const keys = (ops: OperationsOverview) => Object.fromEntries(operationChecks(ops, now).map((check) => [check.key, check.tone]))

describe('operation checks', () => {
  it('treats a missing backup as critical and a day-old one as a warning', () => {
    expect(keys(base).backup).toBe('danger')
    expect(keys({ ...base, last_backup_at: '2026-11-09T06:00:00Z' }).backup).toBe('warning')
    expect(keys({ ...base, last_backup_at: '2026-11-10T03:00:00Z' }).backup).toBe('info')
  })

  it('reports a failure only when it is newer than the last successful backup', () => {
    expect(keys({ ...base, last_backup_at: '2026-11-10T03:00:00Z', last_backup_failure_at: '2026-11-09T03:00:00Z' })['backup-failure']).toBeUndefined()
    expect(keys({ ...base, last_backup_at: '2026-11-09T03:00:00Z', last_backup_failure_at: '2026-11-10T03:00:00Z' })['backup-failure']).toBe('danger')
  })

  it('escalates certificate expiry and warns about the unencrypted profile', () => {
    expect(keys({ ...base, tls_days_left: 60, tls_not_after: '2027-01-09T00:00:00Z' }).tls).toBe('info')
    expect(keys({ ...base, tls_days_left: 14 }).tls).toBe('warning')
    expect(keys({ ...base, tls_days_left: 3 }).tls).toBe('danger')
    expect(keys({ ...base, tls_error: 'сертификат недоступен' }).tls).toBe('danger')
    expect(keys({ ...base, network_profile: 'local-http' }).profile).toBe('warning')
    expect(keys(base).profile).toBeUndefined()
  })

  it('warns before the disk with the database and files fills up', () => {
    const GiB = 1024 ** 3
    const disk = (total: number, used: number) => Object.fromEntries(operationChecks(base, now, { total, used }).map((check) => [check.key, check.tone])).disk
    expect(disk(100 * GiB, 40 * GiB)).toBe('info')
    expect(disk(100 * GiB, 86 * GiB)).toBe('warning')
    expect(disk(100 * GiB, 96 * GiB)).toBe('danger')
    expect(disk(10 * GiB, 9.5 * GiB)).toBe('danger') // меньше 1 ГиБ свободно, хотя занято 95%
    expect(keys(base).disk).toBeUndefined() // размер диска неизвестен — проверки нет
  })
})
