import type { OperationsOverview } from '../api/types'
import { formatBytes } from './chartMath'
import { formatDate } from './dates'

export type OperationCheck = { key: string; tone: 'danger' | 'warning' | 'info'; title: string; text: string }
export type DiskUsage = { total?: number | null; used?: number | null }

const GiB = 1024 ** 3

/**
 * Проверки эксплуатации: свежесть резервной копии, сбои копирования, срок сертификата, сетевой профиль и место на диске
 * с файлами (на сервере это обычно тот же диск, где PostgreSQL: при заполнении останавливаются и запись, и копирование).
 */
export function operationChecks(ops: OperationsOverview, now = new Date(), disk?: DiskUsage): OperationCheck[] {
  const checks: OperationCheck[] = []
  const hours = (value?: string | null) => (value ? (now.getTime() - new Date(value).getTime()) / 3_600_000 : Infinity)
  if (!ops.last_backup_at) {
    checks.push({ key: 'backup', tone: 'danger', title: 'Резервных копий ещё не было', text: 'Настройте ежедневный запуск scripts/backup.sh (docs/backup-restore.md).' })
  } else if (hours(ops.last_backup_at) > 26) {
    checks.push({ key: 'backup', tone: 'warning', title: 'Резервная копия старше суток', text: `Последняя успешная — ${formatDate(ops.last_backup_at, { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' })}. Проверьте расписание копирования.` })
  } else {
    checks.push({ key: 'backup', tone: 'info', title: 'Резервная копия свежая', text: `Последняя — ${formatDate(ops.last_backup_at, { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' })}${ops.last_backup_size_bytes ? `, ${formatBytes(ops.last_backup_size_bytes)}` : ''}.` })
  }
  if (ops.last_backup_failure_at && (!ops.last_backup_at || ops.last_backup_failure_at > ops.last_backup_at)) {
    checks.push({ key: 'backup-failure', tone: 'danger', title: 'Последняя попытка копирования не удалась', text: `${formatDate(ops.last_backup_failure_at, { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' })}: ${ops.last_backup_failure_reason ?? 'см. backups/backup.log'}` })
  }
  if (ops.network_profile === 'local-http') {
    checks.push({ key: 'profile', tone: 'warning', title: 'Профиль local-http: без шифрования', text: 'Допустим только в изолированной сети или на пилоте. Для постоянной работы — профиль https (docs/https.md).' })
  }
  if (ops.tls_error) {
    checks.push({ key: 'tls', tone: 'danger', title: 'Сертификат не прочитан', text: ops.tls_error })
  } else if (ops.tls_days_left != null) {
    const tone = ops.tls_days_left < 7 ? 'danger' : ops.tls_days_left < 21 ? 'warning' : 'info'
    checks.push({ key: 'tls', tone, title: `Сертификат действует ещё ${ops.tls_days_left} дн.`, text: `${ops.tls_names.join(', ') || 'имя не указано'} · до ${formatDate(ops.tls_not_after, { day: 'numeric', month: 'long', year: 'numeric' })}` })
  }
  if (disk?.total && disk.used != null) {
    const free = disk.total - disk.used
    const share = disk.used / disk.total
    const detail = `Свободно ${formatBytes(free)} из ${formatBytes(disk.total)}.`
    if (share >= 0.95 || free < GiB) {
      checks.push({ key: 'disk', tone: 'danger', title: 'Диск почти заполнен', text: `${detail} При заполнении остановятся запись в базу и резервные копии — освободите место (docs/troubleshooting.md, «Место на диске»).` })
    } else if (share >= 0.85) {
      checks.push({ key: 'disk', tone: 'warning', title: 'На диске меньше 15% свободного места', text: `${detail} Проверьте рост копий и вложений, удалите кэш сборки Docker.` })
    } else {
      checks.push({ key: 'disk', tone: 'info', title: 'Места на диске достаточно', text: detail })
    }
  }
  return checks
}
