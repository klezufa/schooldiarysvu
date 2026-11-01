#requires -Version 5.1
<#
.SYNOPSIS
  Резервная копия и восстановление данных «Школьного дневника», запущенного через Docker Compose.

.DESCRIPTION
  backup  — pg_dump базы + архив тома с файлами + manifest.json с контрольными суммами; папка публикуется атомарно.
  restore — проверяет копию, останавливает приложение, пересоздаёт БД, возвращает БД и файлы, запускает приложение.
  Запускать из корня репозитория или через deploy\windows\docker-data.cmd. Данные копии лежат в папке backups\.

.EXAMPLE
  deploy\windows\docker-data.cmd backup
  deploy\windows\docker-data.cmd restore -Archive backups\docker-20261031-201500
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory, Position = 0)][ValidateSet('backup', 'restore')][string]$Action,
    [string]$Archive,
    [string]$EnvFile = '.env',
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$BackupDir = Join-Path $Root 'backups'
$Project = 'schooldiarysvu'   # совпадает с name: в docker-compose.yml
$FilesVolume = "${Project}_files-data"

function Write-Ok([string]$Text) { Write-Host "[ok] $Text" -ForegroundColor Green }
function Write-Warn([string]$Text) { Write-Host "[!] $Text" -ForegroundColor Yellow }

# Простые функции без param(): аргументы вроде -v и -d уходят в docker как есть, а не привязываются к -Verbose/-Debug.
function Invoke-Docker {
    & docker @args
    if ($LASTEXITCODE -ne 0) { throw "docker $($args -join ' ') завершился с кодом $LASTEXITCODE" }
}

function Invoke-Compose {
    $envArguments = @()
    $envPath = if ([System.IO.Path]::IsPathRooted($EnvFile)) { $EnvFile } else { Join-Path $Root $EnvFile }
    if (Test-Path $envPath) { $envArguments = @('--env-file', $envPath) }
    $all = @('compose') + $envArguments + $args
    Push-Location $Root
    try { Invoke-Docker @all } finally { Pop-Location }
}

function Write-MaintenanceEvent([string]$Kind, [string]$Status, $Details) {
    # Журнал эксплуатации для страницы «Система». JSON передаётся в base64: PowerShell 5.1 искажает кавычки
    # в аргументах внешних программ. Ошибка записи не должна скрывать исход самой операции.
    try {
        $json = $Details | ConvertTo-Json -Compress -Depth 4
        $encoded = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($json))
        Invoke-Compose exec -T postgres psql -X -q -U diary -d diary -v ON_ERROR_STOP=1 -c "INSERT INTO maintenance_events (kind, status, details) VALUES ('$Kind', '$Status', convert_from(decode('$encoded', 'base64'), 'UTF8')::jsonb)" | Out-Null
    } catch { Write-Warn "Событие $Kind/$Status не записано на страницу «Система»: $($_.Exception.Message)" }
}

function Get-Sha256([string]$Path) { (Get-FileHash -Path $Path -Algorithm SHA256).Hash.ToLowerInvariant() }

function Invoke-WithRetry {
    # Windows: только что записанные файлы и папки на мгновение блокируются антивирусом/индексатором.
    param([Parameter(Mandatory)][scriptblock]$Script, [int]$Attempts = 8)
    for ($i = 1; $i -le $Attempts; $i++) {
        try { & $Script; return } catch { if ($i -eq $Attempts) { throw }; Start-Sleep -Milliseconds (300 * $i) }
    }
}

function Wait-Healthy {
    $port = if ($env:HTTP_PORT) { $env:HTTP_PORT } else { '8080' }
    $deadline = (Get-Date).AddSeconds(90)
    while ((Get-Date) -lt $deadline) {
        try { if ((Invoke-WebRequest -Uri "http://127.0.0.1:$port/health/ready" -UseBasicParsing -TimeoutSec 3).StatusCode -eq 200) { return $true } } catch { Start-Sleep -Seconds 2 }
    }
    return $false
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Docker не найден. Установите и запустите Docker Desktop.' }
& docker info *> $null
if ($LASTEXITCODE -ne 0) { throw 'Демон Docker не запущен. Запустите Docker Desktop и дождитесь готовности.' }

switch ($Action) {
    'backup' {
        New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null
        $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
        $partial = Join-Path $BackupDir "docker-$stamp.partial"
        $final = Join-Path $BackupDir "docker-$stamp"
        New-Item -ItemType Directory -Force -Path $partial | Out-Null
        try {
            # Дамп снимается внутри контейнера и копируется как файл: редирект stdout в PowerShell 5.1 портит бинарные данные.
            Invoke-Compose exec -T postgres pg_dump -U diary -Fc -f /tmp/diary.dump diary
            Invoke-Compose cp postgres:/tmp/diary.dump (Join-Path $partial 'database.dump')
            Invoke-Compose exec -T postgres rm -f /tmp/diary.dump
            Invoke-Docker run --rm -v "${FilesVolume}:/data:ro" -v "${partial}:/backup" alpine tar czf /backup/files.tgz -C /data .
            $schema = [int](Invoke-Compose exec -T postgres psql -X -At -U diary -d diary -c 'SELECT CASE WHEN dirty THEN -1 ELSE version END FROM schema_migrations')
            if ($schema -lt 0) { throw 'Миграции БД в незавершённом состоянии (dirty) — копирование отменено' }
            $manifest = [ordered]@{
                created_at     = (Get-Date).ToString('o')
                kind           = 'docker-compose'
                schema_version = $schema
                git_commit     = (& git -C $Root rev-parse --short HEAD 2>$null)
                files          = [ordered]@{ 'database.dump' = Get-Sha256 (Join-Path $partial 'database.dump'); 'files.tgz' = Get-Sha256 (Join-Path $partial 'files.tgz') }
            }
            $manifest | ConvertTo-Json -Depth 4 | Set-Content -Path (Join-Path $partial 'manifest.json') -Encoding UTF8
            Invoke-WithRetry { Rename-Item -Path $partial -NewName (Split-Path $final -Leaf) -ErrorAction Stop }
        } catch {
            $reason = $_.Exception.Message
            Remove-Item -Path $partial -Recurse -Force -ErrorAction SilentlyContinue
            Write-MaintenanceEvent 'backup' 'failure' ([ordered]@{ name = "docker-$stamp"; reason = $reason })
            throw
        }
        $size = (Get-ChildItem -Path $final -File | Measure-Object -Property Length -Sum).Sum
        Write-MaintenanceEvent 'backup' 'success' ([ordered]@{ name = "docker-$stamp"; size_bytes = $size; schema_version = $schema; consistent = $false })
        Write-Ok "Резервная копия: $final"
        Write-Warn 'Копия сделана без остановки приложения: файлы могут оказаться новее БД. Для строгой согласованности остановите приложение (docker compose stop backend worker nginx) на время копирования.'
        Write-Warn 'Копия лежит на этом же диске. Скопируйте папку на другой носитель.'
    }
    'restore' {
        if (-not $Archive) { throw 'Укажите копию: docker-data.cmd restore -Archive backups\docker-ГГГГММДД-ЧЧММСС' }
        $source = (Resolve-Path $Archive).Path
        foreach ($name in @('manifest.json', 'database.dump', 'files.tgz')) { if (-not (Test-Path (Join-Path $source $name))) { throw "В копии нет $name" } }
        $manifest = Get-Content -Path (Join-Path $source 'manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        foreach ($name in @('database.dump', 'files.tgz')) {
            if ((Get-Sha256 (Join-Path $source $name)) -ne $manifest.files.$name) { throw "Контрольная сумма $name не совпадает: копия повреждена, восстановление отменено" }
        }
        Write-Ok "Копия проверена (создана $($manifest.created_at))"
        $latest = (Get-ChildItem -Path (Join-Path $Root 'db\migrations') -Filter '*.up.sql' | ForEach-Object { [int]($_.Name.Split('_')[0]) } | Measure-Object -Maximum).Maximum
        $backupSchema = if ($manifest.PSObject.Properties['schema_version']) { [int]$manifest.schema_version } else { 0 }
        if ($backupSchema -gt $latest) { throw "Схема копии ($backupSchema) новее, чем знает этот код ($latest): сначала обновите код до версии, с которой сделана копия" }
        if (-not $Force) {
            Write-Warn 'Текущая база и файлы в Docker-томах будут ПОЛНОСТЬЮ заменены данными из копии. Перед этим будет сделана страховочная копия.'
            if ((Read-Host -Prompt 'Введите YES для продолжения') -ne 'YES') { throw 'Отменено пользователем' }
        }
        Invoke-Compose up -d postgres
        Write-Ok 'Страховочная копия текущего состояния'
        & $PSCommandPath backup -EnvFile $EnvFile
        $safety = Get-ChildItem -Path $BackupDir -Directory -Filter 'docker-*' | Where-Object { $_.Name -notlike '*.partial' } | Sort-Object Name | Select-Object -Last 1
        $name = Split-Path $source -Leaf
        try {
            Invoke-Compose stop nginx worker backend
            Invoke-Compose cp (Join-Path $source 'database.dump') postgres:/tmp/restore.dump
            # Копия восстанавливается во временную БД: если pg_restore упадёт, рабочая база останется нетронутой.
            Invoke-Compose exec -T postgres psql -U diary -d postgres -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS diary_restore' -c "CREATE DATABASE diary_restore ENCODING 'UTF8' TEMPLATE template0"
            Invoke-Compose exec -T postgres pg_restore -U diary -d diary_restore --no-owner --exit-on-error --single-transaction /tmp/restore.dump
            Invoke-Compose exec -T postgres psql -U diary -d postgres -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS diary WITH (FORCE)' -c 'ALTER DATABASE diary_restore RENAME TO diary'
            # Сессии из копии могли быть отозваны уже после её создания: завершаем все, пользователи входят заново.
            Invoke-Compose exec -T postgres psql -U diary -d diary -v ON_ERROR_STOP=1 -c 'UPDATE sessions SET revoked_at = now() WHERE revoked_at IS NULL'
            Invoke-Compose exec -T postgres rm -f /tmp/restore.dump
            # Права 65532 — пользователь nonroot в образе backend.
            Invoke-Docker run --rm -v "${FilesVolume}:/data" -v "${source}:/backup:ro" alpine sh -c 'rm -rf /data/* /data/.[!.]* 2>/dev/null; tar xzf /backup/files.tgz -C /data && chown -R 65532:65532 /data'
            Invoke-Compose up -d
            if (-not (Wait-Healthy)) { throw 'После восстановления приложение не стало готовым. Смотрите: docker compose logs backend' }
        } catch {
            $reason = $_.Exception.Message
            Invoke-Compose up -d
            Write-MaintenanceEvent 'restore' 'failure' ([ordered]@{ name = $name; reason = $reason })
            Write-Warn "Восстановление не завершено. Состояние до восстановления сохранено в $($safety.FullName)"
            throw
        }
        # Событие о страховочной копии записывалось в прежнюю базу, которую заменила копия: повторяем его,
        # иначе страница «Система» покажет устаревшую дату последней резервной копии.
        $safetyManifest = Get-Content -Path (Join-Path $safety.FullName 'manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
        Write-MaintenanceEvent 'backup' 'success' ([ordered]@{ name = $safety.Name; schema_version = [int]$safetyManifest.schema_version; consistent = $false })
        Write-MaintenanceEvent 'restore' 'success' ([ordered]@{ name = $name; schema_version = $backupSchema })
        Write-Ok 'Восстановление завершено. Войдите в систему и проверьте журнал и вложения.'
    }
}
