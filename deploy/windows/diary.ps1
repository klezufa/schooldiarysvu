#requires -Version 5.1
<#
.SYNOPSIS
  Локальный запуск «Школьного дневника» на Windows без Docker: приватный PostgreSQL + server.exe + worker.exe.

.DESCRIPTION
  Все данные, секреты, логи и бинарники лежат в каталоге .local внутри репозитория (он в .gitignore).
  Системная служба PostgreSQL не используется и не изменяется, права администратора не нужны.
  Режим предназначен для тестирования на одном компьютере (профиль local-http, привязка к 127.0.0.1).
  Это НЕ защищённый production: см. docs/windows-local.md.

.EXAMPLE
  deploy\windows\diary.cmd doctor
  deploy\windows\diary.cmd setup
  deploy\windows\diary.cmd start
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('help', 'doctor', 'setup', 'start', 'stop', 'restart', 'status', 'logs', 'create-admin', 'seed-demo', 'backup', 'restore', 'reset')]
    [string]$Command = 'help',
    [string]$Archive,
    [switch]$Quiesce,
    [switch]$Force,
    # seed-demo -Rich: расширенный набор (25 учеников, 7 предметов, оценки и посещаемость с начала года) для проверки графиков.
    [switch]$Rich
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$Local = Join-Path $Root '.local'
$BinDir = Join-Path $Local 'bin'
$RunDir = Join-Path $Local 'run'
$LogDir = Join-Path $Local 'logs'
$PgData = Join-Path $Local 'pgdata'
$FilesDir = Join-Path $Local 'files'
$BackupDir = Join-Path $Local 'backups'
$ConfigPath = Join-Path $Local 'config.env'
$WebDist = Join-Path $Root 'web\dist'

function Write-Step([string]$Text) { Write-Host "==> $Text" -ForegroundColor Cyan }
function Write-Ok([string]$Text) { Write-Host "[ok] $Text" -ForegroundColor Green }
function Write-Warn([string]$Text) { Write-Host "[!] $Text" -ForegroundColor Yellow }
function Write-Fail([string]$Text) { Write-Host "[x] $Text" -ForegroundColor Red }

function Invoke-Native {
    # Запускает внешнюю программу и останавливает скрипт при ненулевом коде возврата.
    param([Parameter(Mandatory)][string]$File, [string[]]$Arguments = @())
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Команда завершилась с кодом ${LASTEXITCODE}: $File $($Arguments -join ' ')" }
}

function Invoke-WithRetry {
    # Windows: только что записанные файлы и папки на мгновение блокируются антивирусом/индексатором.
    param([Parameter(Mandatory)][scriptblock]$Action, [int]$Attempts = 8)
    for ($i = 1; $i -le $Attempts; $i++) {
        try { & $Action; return } catch { if ($i -eq $Attempts) { throw }; Start-Sleep -Milliseconds (300 * $i) }
    }
}

function New-Secret([int]$Length = 24) {
    $alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
    $bytes = New-Object byte[] $Length
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    -join ($bytes | ForEach-Object { $alphabet[$_ % $alphabet.Length] })
}

function Restrict-ToCurrentUser([string]$Path) {
    # Каталог или файл с секретами доступен только текущему пользователю. Права выдаются по SID, а для каталогов
    # с флагами наследования (OI)(CI): без них вложенные файлы остались бы совсем без прав доступа.
    $sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $grant = if (Test-Path -Path $Path -PathType Container) { "*${sid}:(OI)(CI)F" } else { "*${sid}:F" }
    & icacls.exe $Path /inheritance:r /grant:r $grant | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Не удалось ограничить доступ к $Path" }
}

function Find-Tool {
    param([string]$Name, [string[]]$Candidates)
    $onPath = Get-Command $Name -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }
    foreach ($pattern in $Candidates) {
        $hit = Get-ChildItem -Path $pattern -ErrorAction SilentlyContinue | Sort-Object FullName -Descending | Select-Object -First 1
        if ($hit) { return $hit.FullName }
    }
    return $null
}

function Get-Tools {
    $pgDirs = @(
        (Join-Path $Root '.tools\postgresql\pgsql\bin'),
        "$env:ProgramFiles\PostgreSQL\*\bin"
    )
    if ($env:DIARY_PG_BIN) { $pgDirs = @($env:DIARY_PG_BIN) + $pgDirs }
    function Find-Pg([string]$Exe) {
        foreach ($dir in $pgDirs) {
            $hit = Get-ChildItem -Path (Join-Path $dir $Exe) -ErrorAction SilentlyContinue | Sort-Object FullName -Descending | Select-Object -First 1
            if ($hit) { return $hit.FullName }
        }
        $onPath = Get-Command $Exe -ErrorAction SilentlyContinue
        if ($onPath) { return $onPath.Source }
        return $null
    }
    [pscustomobject]@{
        Go       = Find-Tool 'go' @((Join-Path $Root '.tools\go\bin\go.exe'), "$env:ProgramFiles\Go\bin\go.exe")
        Node     = Find-Tool 'node' @("$env:ProgramFiles\nodejs\node.exe")
        Npm      = Find-Tool 'npm.cmd' @("$env:ProgramFiles\nodejs\npm.cmd")
        Initdb   = Find-Pg 'initdb.exe'
        PgCtl    = Find-Pg 'pg_ctl.exe'
        Psql     = Find-Pg 'psql.exe'
        PgDump   = Find-Pg 'pg_dump.exe'
        PgRestore = Find-Pg 'pg_restore.exe'
    }
}

function Read-Config {
    $config = @{}
    if (Test-Path $ConfigPath) {
        foreach ($line in Get-Content -Path $ConfigPath -Encoding UTF8) {
            if ($line -match '^\s*([A-Z0-9_]+)\s*=\s*(.*)$') { $config[$Matches[1]] = $Matches[2].Trim() }
        }
    }
    return $config
}

function Save-Config([hashtable]$Config) {
    $lines = $Config.GetEnumerator() | Sort-Object Name | ForEach-Object { "$($_.Name)=$($_.Value)" }
    Set-Content -Path $ConfigPath -Value $lines -Encoding UTF8
    Restrict-ToCurrentUser $ConfigPath
}

function Get-Setting([hashtable]$Config, [string]$Name, [string]$Default) {
    if ($Config.ContainsKey($Name) -and $Config[$Name]) { return $Config[$Name] }
    return $Default
}

function Get-Settings {
    $config = Read-Config
    if (-not $config.ContainsKey('POSTGRES_PASSWORD')) { throw 'Не найден .local\config.env. Сначала выполните: diary.cmd setup' }
    $pgPort = Get-Setting $config 'DIARY_PG_PORT' '55440'
    $httpPort = Get-Setting $config 'DIARY_HTTP_PORT' '8090'
    $bind = Get-Setting $config 'DIARY_BIND_ADDR' '127.0.0.1'
    [pscustomobject]@{
        Config       = $config
        PgPort       = $pgPort
        HttpPort     = $httpPort
        BindAddr     = $bind
        Password     = $config['POSTGRES_PASSWORD']
        DatabaseUrl  = "postgres://diary:$($config['POSTGRES_PASSWORD'])@127.0.0.1:${pgPort}/diary?sslmode=disable"
        HealthUrl    = "http://127.0.0.1:${httpPort}/health/ready"
        SiteUrl      = "http://127.0.0.1:${httpPort}/"
    }
}

function Set-AppEnvironment($Settings) {
    # Переменные окружения наследуются запускаемыми процессами (server, worker, утилиты).
    $env:APP_ENV = 'development'
    $env:NETWORK_PROFILE = 'local-http'
    $env:DATABASE_URL = $Settings.DatabaseUrl
    $env:HTTP_ADDR = "$($Settings.BindAddr):$($Settings.HttpPort)"
    $env:SESSION_COOKIE_SECURE = 'false'
    $env:FILE_STORAGE_PATH = $FilesDir
    $env:STATIC_DIR = $WebDist
    $env:SCHOOL_TIMEZONE = Get-Setting $Settings.Config 'SCHOOL_TIMEZONE' 'Europe/Moscow'
}

function Get-PidFile([string]$Name) { Join-Path $RunDir "$Name.pid" }

function Get-RunningProcess([string]$Name) {
    $pidFile = Get-PidFile $Name
    if (-not (Test-Path $pidFile)) { return $null }
    $recorded = [int](Get-Content -Path $pidFile -Raw)
    $process = Get-Process -Id $recorded -ErrorAction SilentlyContinue
    if ($process -and $process.ProcessName -eq $Name) { return $process }
    Remove-Item -Path $pidFile -Force -ErrorAction SilentlyContinue
    return $null
}

function Start-DiaryProcess([string]$Name) {
    if (Get-RunningProcess $Name) { Write-Ok "$Name уже запущен"; return }
    $exe = Join-Path $BinDir "$Name.exe"
    if (-not (Test-Path $exe)) { throw "Нет $exe. Выполните: diary.cmd setup" }
    $process = Start-Process -FilePath $exe -WorkingDirectory $Root -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $LogDir "$Name.log") -RedirectStandardError (Join-Path $LogDir "$Name.err.log")
    Set-Content -Path (Get-PidFile $Name) -Value $process.Id
    Write-Ok "$Name запущен (PID $($process.Id))"
}

function Stop-DiaryProcess([string]$Name) {
    $process = Get-RunningProcess $Name
    if (-not $process) { return }
    Stop-Process -Id $process.Id -Force
    $process.WaitForExit(10000) | Out-Null
    Remove-Item -Path (Get-PidFile $Name) -Force -ErrorAction SilentlyContinue
    Write-Ok "$Name остановлен"
}

function Test-PostgresRunning($Tools) {
    if (-not (Test-Path (Join-Path $PgData 'postmaster.pid'))) { return $false }
    & $Tools.PgCtl status -D $PgData *> $null
    return ($LASTEXITCODE -eq 0)
}

function Start-Postgres($Tools, $Settings) {
    if (Test-PostgresRunning $Tools) { Write-Ok "PostgreSQL уже запущен (порт $($Settings.PgPort))"; return }
    # pg_ctl запускается через Start-Process с выводом в файлы: иначе долгоживущий postgres наследует дескрипторы
    # консоли или канала и вызывающая сторона (терминал, CI) не может завершиться.
    $arguments = "start -D `"$PgData`" -l `"$(Join-Path $LogDir 'postgres.log')`" -w -t 60 -o `"-p $($Settings.PgPort) -c listen_addresses=127.0.0.1`""
    $process = Start-Process -FilePath $Tools.PgCtl -ArgumentList $arguments -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $LogDir 'pg_ctl.out.log') -RedirectStandardError (Join-Path $LogDir 'pg_ctl.err.log')
    # Не используем Start-Process -Wait: в Windows PowerShell 5.1 он ждёт и потомков, то есть сам долгоживущий postgres.
    $null = $process.Handle
    if (-not $process.WaitForExit(90000)) { throw 'pg_ctl не завершился за 90 секунд' }
    if ($process.ExitCode -ne 0) {
        Get-Content -Path (Join-Path $LogDir 'postgres.log') -Tail 15 -ErrorAction SilentlyContinue
        throw "PostgreSQL не запустился (код $($process.ExitCode)). Журнал: .local\logs\postgres.log"
    }
    Write-Ok "PostgreSQL запущен (127.0.0.1:$($Settings.PgPort))"
}

function Stop-Postgres($Tools) {
    if (-not (Test-PostgresRunning $Tools)) { return }
    Invoke-Native $Tools.PgCtl @('stop', '-D', $PgData, '-m', 'fast', '-w')
    Write-Ok 'PostgreSQL остановлен'
}

function Wait-Ready($Settings, [int]$Seconds = 60) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri $Settings.HealthUrl -UseBasicParsing -TimeoutSec 3
            if ($response.StatusCode -eq 200) { return $true }
        } catch { Start-Sleep -Milliseconds 700 }
    }
    return $false
}

function Invoke-Psql($Tools, $Settings, [string]$Database, [string]$Sql) {
    # Пароль передаётся через PGPASSWORD; прежнее значение восстанавливается, чтобы не сломать вызывающий код
    # (например, pg_restore, запущенный после psql в том же блоке). Флаг -w: никогда не ждать ввода пароля.
    $previous = $env:PGPASSWORD
    $env:PGPASSWORD = $Settings.Password
    try { & $Tools.Psql -w -h 127.0.0.1 -p $Settings.PgPort -U diary -d $Database -v ON_ERROR_STOP=1 -At -c $Sql }
    finally { if ($null -eq $previous) { Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue } else { $env:PGPASSWORD = $previous } }
    if ($LASTEXITCODE -ne 0) { throw "psql завершился с ошибкой: $Sql" }
}

function Write-MaintenanceEvent($Tools, $Settings, [string]$Kind, [string]$Status, $Details) {
    # Журнал эксплуатации для страницы «Система». JSON передаётся в base64: PowerShell 5.1 искажает кавычки
    # в аргументах внешних программ. Ошибка записи не должна скрывать исход самой операции.
    try {
        $json = $Details | ConvertTo-Json -Compress -Depth 4
        $encoded = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($json))
        Invoke-Psql $Tools $Settings 'diary' "INSERT INTO maintenance_events (kind, status, details) VALUES ('$Kind', '$Status', convert_from(decode('$encoded', 'base64'), 'UTF8')::jsonb)" | Out-Null
    } catch { Write-Warn "Событие $Kind/$Status не записано на страницу «Система»: $($_.Exception.Message)" }
}

function Assert-Tools($Tools, [string[]]$Required) {
    $missing = @()
    foreach ($name in $Required) { if (-not $Tools.$name) { $missing += $name } }
    if ($missing.Count) {
        Show-Doctor $Tools | Out-Null
        throw "Не найдены инструменты: $($missing -join ', '). Подсказки по установке выше (команда doctor)."
    }
}

function Show-Doctor($Tools) {
    Write-Step 'Проверка инструментов'
    $hints = @{
        Go        = 'winget install --id GoLang.Go -e   (нужен Go 1.26 или новее)'
        Node      = 'winget install --id OpenJS.NodeJS.LTS -e'
        Npm       = 'устанавливается вместе с Node.js'
        Initdb    = 'winget install --id PostgreSQL.PostgreSQL.17 -e   (служба не нужна; достаточно файлов bin). Либо путь в DIARY_PG_BIN'
        PgCtl     = 'см. Initdb'
        Psql      = 'см. Initdb'
        PgDump    = 'см. Initdb'
        PgRestore = 'см. Initdb'
    }
    $ok = $true
    foreach ($property in $Tools.PSObject.Properties) {
        if ($property.Value) { Write-Ok ("{0,-9} {1}" -f $property.Name, $property.Value) }
        else { Write-Fail ("{0,-9} не найден. {1}" -f $property.Name, $hints[$property.Name]); $ok = $false }
    }
    if ($Tools.Go) {
        $version = (& $Tools.Go version) -replace 'go version go', '' -replace ' .*', ''
        if ([version]($version -replace '[^0-9.].*', '') -lt [version]'1.26.0') { Write-Fail "Go $version слишком старый, нужен 1.25+"; $ok = $false }
    }
    return $ok
}

function Invoke-Setup {
    $tools = Get-Tools
    Assert-Tools $tools @('Go', 'Npm', 'Initdb', 'PgCtl', 'Psql')
    foreach ($dir in @($Local, $BinDir, $RunDir, $LogDir, $FilesDir, $BackupDir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    # Каталог .local содержит секреты и данные: доступ только текущему пользователю.
    Restrict-ToCurrentUser $Local

    $config = Read-Config
    if (-not $config.ContainsKey('POSTGRES_PASSWORD')) {
        $config['POSTGRES_PASSWORD'] = New-Secret 28
        $config['DIARY_PG_PORT'] = '55440'
        $config['DIARY_HTTP_PORT'] = '8090'
        $config['DIARY_BIND_ADDR'] = '127.0.0.1'
        Save-Config $config
        Write-Ok 'Создан .local\config.env (пароль БД сгенерирован, в репозиторий не попадает)'
    }
    $settings = Get-Settings

    Write-Step 'Сборка frontend (npm ci && npm run build)'
    Push-Location (Join-Path $Root 'web')
    try {
        # npm ci удаляет node_modules целиком, поэтому выполняем его только при изменении package-lock.json
        # (и не мешаем запущенному dev-серверу Vite, который держит файлы открытыми).
        $installed = Join-Path $Root 'web\node_modules\.package-lock.json'
        $lockFile = Join-Path $Root 'web\package-lock.json'
        if ((Test-Path $installed) -and ((Get-Item $installed).LastWriteTime -ge (Get-Item $lockFile).LastWriteTime)) {
            Write-Ok 'Зависимости frontend актуальны, npm ci пропущен'
        } else {
            try { Invoke-Native $tools.Npm @('ci', '--no-audit', '--no-fund') }
            catch {
                Write-Warn 'npm ci не смог заменить node_modules. Чаще всего их держит открытыми запущенный dev-сервер Vite (npm run dev) или редактор. Остановите их и повторите setup.'
                throw
            }
        }
        Invoke-Native $tools.Npm @('run', 'build')
    } finally { Pop-Location }

    # Работающий server.exe/worker.exe нельзя перезаписать: останавливаем приложение (PostgreSQL остаётся).
    $wasRunning = [bool](Get-RunningProcess 'server')
    Stop-DiaryProcess 'worker'; Stop-DiaryProcess 'server'
    Write-Step 'Сборка backend (go build)'
    Push-Location $Root
    try {
        foreach ($name in @('server', 'worker', 'seed', 'bootstrap-admin')) {
            Invoke-Native $tools.Go @('build', '-trimpath', '-o', (Join-Path $BinDir "$name.exe"), "./cmd/$name")
        }
    } finally { Pop-Location }
    Write-Ok "Бинарники собраны в $BinDir"

    Write-Step 'Подготовка PostgreSQL (отдельный кластер в .local\pgdata)'
    if (-not (Test-Path (Join-Path $PgData 'PG_VERSION'))) {
        $pwFile = Join-Path $RunDir 'pg.pwfile'
        # initdb читает пароль суперпользователя из файла; файл удаляется сразу после инициализации.
        [System.IO.File]::WriteAllText($pwFile, $settings.Password, (New-Object System.Text.UTF8Encoding($false)))
        try { Invoke-Native $tools.Initdb @('-D', $PgData, '-U', 'diary', '-E', 'UTF8', '--auth=scram-sha-256', "--pwfile=$pwFile") }
        finally { Remove-Item -Path $pwFile -Force -ErrorAction SilentlyContinue }
        Write-Ok 'Кластер PostgreSQL инициализирован'
    } else { Write-Ok 'Кластер PostgreSQL уже существует' }
    Start-Postgres $tools $settings
    $exists = Invoke-Psql $tools $settings 'postgres' "SELECT 1 FROM pg_database WHERE datname = 'diary'"
    if (-not $exists) { Invoke-Psql $tools $settings 'postgres' 'CREATE DATABASE diary ENCODING ''UTF8'' TEMPLATE template0' | Out-Null; Write-Ok 'База diary создана' }
    Write-Host ''
    if ($wasRunning) { Write-Warn 'Приложение было остановлено для пересборки. Запустите его снова: diary.cmd start' }
    Write-Ok 'Готово. Дальше: diary.cmd start, затем diary.cmd create-admin (или seed-demo для демо-данных).'
}

function Invoke-Start {
    $tools = Get-Tools
    Assert-Tools $tools @('PgCtl', 'Psql')
    $settings = Get-Settings
    if (-not (Test-Path (Join-Path $WebDist 'index.html'))) { throw 'Нет web\dist. Выполните: diary.cmd setup' }
    Set-AppEnvironment $settings
    Start-Postgres $tools $settings
    Start-DiaryProcess 'server'
    if (-not (Wait-Ready $settings)) {
        Write-Fail 'Сервер не стал готовым за 60 секунд. Последние строки журнала:'
        Get-Content -Path (Join-Path $LogDir 'server.err.log') -Tail 15 -ErrorAction SilentlyContinue
        Get-Content -Path (Join-Path $LogDir 'server.log') -Tail 15 -ErrorAction SilentlyContinue
        throw 'Запуск не удался'
    }
    Start-DiaryProcess 'worker'
    Write-Host ''
    Write-Ok "Дневник работает: $($settings.SiteUrl)"
    if ($settings.BindAddr -ne '127.0.0.1') { Write-Warn "Сервер слушает $($settings.BindAddr): он доступен из сети по НЕЗАШИФРОВАННОМУ HTTP." }
}

function Invoke-Stop {
    $tools = Get-Tools
    Stop-DiaryProcess 'worker'
    Stop-DiaryProcess 'server'
    if ($tools.PgCtl -and (Test-Path (Join-Path $PgData 'PG_VERSION'))) { Stop-Postgres $tools }
}

function Invoke-Status {
    $tools = Get-Tools
    $settings = Get-Settings
    foreach ($name in @('server', 'worker')) {
        $process = Get-RunningProcess $name
        if ($process) { Write-Ok "$name работает (PID $($process.Id))" } else { Write-Warn "$name остановлен" }
    }
    if ($tools.PgCtl -and (Test-PostgresRunning $tools)) { Write-Ok "PostgreSQL работает (порт $($settings.PgPort))" } else { Write-Warn 'PostgreSQL остановлен' }
    try {
        $response = Invoke-WebRequest -Uri $settings.HealthUrl -UseBasicParsing -TimeoutSec 3
        Write-Ok "/health/ready: $($response.StatusCode) $($response.Content)"
    } catch { Write-Warn '/health/ready недоступен' }
    Write-Host "Адрес: $($settings.SiteUrl)"
}

function Invoke-Logs {
    foreach ($name in @('server.log', 'server.err.log', 'worker.log', 'worker.err.log', 'postgres.log')) {
        $path = Join-Path $LogDir $name
        if ((Test-Path $path) -and (Get-Item $path).Length -gt 0) {
            Write-Host "----- $name (последние 20 строк) -----" -ForegroundColor Cyan
            Get-Content -Path $path -Tail 20
        }
    }
}

function ConvertTo-PlainText([System.Security.SecureString]$Secure) {
    $pointer = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($Secure)
    try { return [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer) }
    finally { [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
}

function Read-NewPassword([string]$Prompt, [string]$EnvName) {
    # Для автоматизации пароль можно передать через переменную окружения; по умолчанию он вводится скрыто.
    $fromEnv = [Environment]::GetEnvironmentVariable($EnvName)
    if ($fromEnv) {
        if ($fromEnv.Length -lt 12) { throw "${EnvName}: пароль должен содержать не менее 12 символов" }
        return $fromEnv
    }
    $first = ConvertTo-PlainText (Read-Host -Prompt $Prompt -AsSecureString)
    $second = ConvertTo-PlainText (Read-Host -Prompt 'Повторите пароль' -AsSecureString)
    if ($first -ne $second) { throw 'Пароли не совпадают' }
    if ($first.Length -lt 12) { throw 'Пароль должен содержать не менее 12 символов' }
    return $first
}

function Invoke-CreateAdmin {
    $tools = Get-Tools
    Assert-Tools $tools @('PgCtl', 'Psql')
    $settings = Get-Settings
    Set-AppEnvironment $settings
    Start-Postgres $tools $settings
    $login = if ($env:DIARY_ADMIN_LOGIN) { $env:DIARY_ADMIN_LOGIN } else { Read-Host -Prompt 'Логин системного администратора (например, sysadmin)' }
    $name = if ($env:DIARY_ADMIN_NAME) { $env:DIARY_ADMIN_NAME } else { Read-Host -Prompt 'Отображаемое имя' }
    $env:BOOTSTRAP_ADMIN_LOGIN = $login
    $env:BOOTSTRAP_ADMIN_DISPLAY_NAME = $name
    $env:BOOTSTRAP_ADMIN_PASSWORD = Read-NewPassword 'Временный пароль (не менее 12 символов; при первом входе его потребуют сменить)' 'DIARY_ADMIN_PASSWORD'
    try { Invoke-Native (Join-Path $BinDir 'bootstrap-admin.exe') }
    finally { Remove-Item Env:BOOTSTRAP_ADMIN_PASSWORD, Env:BOOTSTRAP_ADMIN_LOGIN, Env:BOOTSTRAP_ADMIN_DISPLAY_NAME -ErrorAction SilentlyContinue }
    Write-Ok 'Администратор создан. Войдите и смените временный пароль.'
}

function Invoke-SeedDemo {
    $tools = Get-Tools
    Assert-Tools $tools @('PgCtl', 'Psql')
    $settings = Get-Settings
    Set-AppEnvironment $settings
    Start-Postgres $tools $settings
    # Схему БД создаёт server при запуске (миграции), поэтому до первого start демо-данные загрузить нельзя.
    if (-not (Invoke-Psql $tools $settings 'diary' "SELECT to_regclass('public.school_settings')")) { throw 'Схема БД ещё не создана. Сначала выполните: diary.cmd start (миграции применяются при запуске), затем повторите seed-demo.' }
    $env:ALLOW_DEMO_SEED = 'true'
    if ($Rich) { $env:DEMO_DATASET = 'rich' }
    $env:DEMO_PASSWORD = Read-NewPassword 'Временный пароль для ВСЕХ демо-пользователей (не менее 12 символов)' 'DIARY_DEMO_PASSWORD'
    try { Invoke-Native (Join-Path $BinDir 'seed.exe') }
    finally { Remove-Item Env:DEMO_PASSWORD, Env:ALLOW_DEMO_SEED, Env:DEMO_DATASET -ErrorAction SilentlyContinue }
    Write-Ok 'Демо-данные созданы. Логины: demo-teacher1, demo-student1, demo-parent1 и др. (см. docs/windows-local.md)'
}

function Get-FileHashHex([string]$Path) { (Get-FileHash -Path $Path -Algorithm SHA256).Hash.ToLowerInvariant() }

function Invoke-Backup {
    $tools = Get-Tools
    Assert-Tools $tools @('PgCtl', 'Psql', 'PgDump')
    $settings = Get-Settings
    $wasRunning = [bool](Get-RunningProcess 'server')
    if ($Quiesce -and $wasRunning) { Write-Step 'Останавливаю приложение для согласованной копии'; Stop-DiaryProcess 'worker'; Stop-DiaryProcess 'server' }
    Start-Postgres $tools $settings
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $partial = Join-Path $BackupDir "diary-$stamp.partial"
    $final = Join-Path $BackupDir "diary-$stamp"
    New-Item -ItemType Directory -Force -Path $partial | Out-Null
    try {
        Write-Step 'pg_dump'
        $env:PGPASSWORD = $settings.Password
        try { Invoke-Native $tools.PgDump @('-w', '-h', '127.0.0.1', '-p', $settings.PgPort, '-U', 'diary', '-d', 'diary', '-Fc', '-f', (Join-Path $partial 'database.dump')) }
        finally { Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue }
        Write-Step 'Файлы вложений'
        $filesZip = Join-Path $partial 'files.zip'
        if (Get-ChildItem -Path $FilesDir -Recurse -File -ErrorAction SilentlyContinue) { Compress-Archive -Path (Join-Path $FilesDir '*') -DestinationPath $filesZip -CompressionLevel Optimal }
        else { Add-Type -AssemblyName System.IO.Compression.FileSystem; [System.IO.Compression.ZipFile]::Open($filesZip, 'Create').Dispose() }
        $migration = Invoke-Psql $tools $settings 'diary' 'SELECT max(version)::text FROM schema_migrations'
        $commit = (& git -C $Root rev-parse --short HEAD 2>$null)
        $manifest = [ordered]@{
            created_at     = (Get-Date).ToString('o')
            consistent     = [bool]$Quiesce
            schema_version = $migration
            git_commit     = $commit
            files          = [ordered]@{ 'database.dump' = Get-FileHashHex (Join-Path $partial 'database.dump'); 'files.zip' = Get-FileHashHex $filesZip }
        }
        $manifest | ConvertTo-Json -Depth 4 | Set-Content -Path (Join-Path $partial 'manifest.json') -Encoding UTF8
        # Публикация копии атомарна: пока она не завершена, каталог имеет суффикс .partial.
        Invoke-WithRetry { Rename-Item -Path $partial -NewName (Split-Path $final -Leaf) -ErrorAction Stop }
    } catch {
        $reason = $_.Exception.Message
        Remove-Item -Path $partial -Recurse -Force -ErrorAction SilentlyContinue
        Write-MaintenanceEvent $tools $settings 'backup' 'failure' ([ordered]@{ name = "diary-$stamp"; reason = $reason })
        throw
    } finally {
        if ($Quiesce -and $wasRunning) { Invoke-Start }
    }
    $size = (Get-ChildItem -Path $final -File | Measure-Object -Property Length -Sum).Sum
    Write-MaintenanceEvent $tools $settings 'backup' 'success' ([ordered]@{ name = "diary-$stamp"; size_bytes = $size; schema_version = [int]$migration; consistent = [bool]$Quiesce })
    $keep = [int](Get-Setting $settings.Config 'DIARY_BACKUP_KEEP' '14')
    Get-ChildItem -Path $BackupDir -Directory -Filter 'diary-*' | Where-Object { $_.Name -notlike '*.partial' } | Sort-Object Name -Descending | Select-Object -Skip $keep | Remove-Item -Recurse -Force
    Write-Ok "Резервная копия: $final"
    if (-not $Quiesce) { Write-Warn 'Копия сделана без остановки приложения: файлы могут быть новее БД. Для согласованной копии используйте -Quiesce.' }
    Write-Warn 'Эта копия лежит на том же диске. Скопируйте папку на другой носитель, иначе это не защита от потери диска.'
}

function Invoke-Restore {
    if (-not $Archive) { throw 'Укажите копию: diary.cmd restore -Archive .local\backups\diary-ГГГГММДД-ЧЧММСС' }
    $source = (Resolve-Path $Archive).Path
    foreach ($name in @('manifest.json', 'database.dump', 'files.zip')) { if (-not (Test-Path (Join-Path $source $name))) { throw "В копии нет $name" } }
    $manifest = Get-Content -Path (Join-Path $source 'manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    foreach ($name in @('database.dump', 'files.zip')) {
        if ((Get-FileHashHex (Join-Path $source $name)) -ne $manifest.files.$name) { throw "Контрольная сумма $name не совпадает: копия повреждена, восстановление отменено" }
    }
    Write-Ok "Копия проверена (создана $($manifest.created_at), схема $($manifest.schema_version))"
    $latest = (Get-ChildItem -Path (Join-Path $Root 'db\migrations') -Filter '*.up.sql' | ForEach-Object { [int]($_.Name.Split('_')[0]) } | Measure-Object -Maximum).Maximum
    if ([int]$manifest.schema_version -gt $latest) { throw "Схема копии ($($manifest.schema_version)) новее, чем знает этот код ($latest): сначала обновите код до версии, с которой сделана копия" }
    if (-not $Force) {
        Write-Warn 'Текущая база diary и файлы будут ПОЛНОСТЬЮ заменены данными из копии.'
        if ((Read-Host -Prompt 'Введите YES для продолжения') -ne 'YES') { throw 'Отменено пользователем' }
    }
    $tools = Get-Tools
    Assert-Tools $tools @('PgCtl', 'Psql', 'PgRestore')
    $settings = Get-Settings
    Stop-DiaryProcess 'worker'; Stop-DiaryProcess 'server'
    Start-Postgres $tools $settings
    $safety = Join-Path $BackupDir ("before-restore-" + (Get-Date -Format 'yyyyMMdd-HHmmss'))
    New-Item -ItemType Directory -Force -Path $safety | Out-Null
    if (Get-ChildItem -Path $FilesDir -Recurse -File -ErrorAction SilentlyContinue) { Compress-Archive -Path (Join-Path $FilesDir '*') -DestinationPath (Join-Path $safety 'files.zip') }
    $name = Split-Path $source -Leaf
    try {
        $env:PGPASSWORD = $settings.Password
        try {
            Invoke-Native $tools.PgDump @('-w', '-h', '127.0.0.1', '-p', $settings.PgPort, '-U', 'diary', '-d', 'diary', '-Fc', '-f', (Join-Path $safety 'database.dump'))
            Write-Ok "Текущее состояние сохранено в $safety (на случай ошибки)"
            # Копия восстанавливается во временную БД: если pg_restore упадёт, рабочая база останется нетронутой.
            Invoke-Psql $tools $settings 'postgres' 'DROP DATABASE IF EXISTS diary_restore' | Out-Null
            Invoke-Psql $tools $settings 'postgres' 'CREATE DATABASE diary_restore ENCODING ''UTF8'' TEMPLATE template0' | Out-Null
            Invoke-Native $tools.PgRestore @('-w', '-h', '127.0.0.1', '-p', $settings.PgPort, '-U', 'diary', '-d', 'diary_restore', '--no-owner', '--exit-on-error', '--single-transaction', (Join-Path $source 'database.dump'))
            Invoke-Psql $tools $settings 'postgres' 'DROP DATABASE IF EXISTS diary WITH (FORCE)' | Out-Null
            Invoke-Psql $tools $settings 'postgres' 'ALTER DATABASE diary_restore RENAME TO diary' | Out-Null
            # Сессии из копии могли быть отозваны уже после её создания: завершаем все, пользователи входят заново.
            Invoke-Psql $tools $settings 'diary' 'UPDATE sessions SET revoked_at = now() WHERE revoked_at IS NULL' | Out-Null
        } finally { Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue }
        Get-ChildItem -Path $FilesDir -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
        Expand-Archive -Path (Join-Path $source 'files.zip') -DestinationPath $FilesDir -Force
        Invoke-Start
    } catch {
        $reason = $_.Exception.Message
        Write-MaintenanceEvent $tools $settings 'restore' 'failure' ([ordered]@{ name = $name; reason = $reason })
        Write-Fail "Восстановление не завершено. Состояние до восстановления сохранено в $safety"
        throw
    }
    Write-MaintenanceEvent $tools $settings 'restore' 'success' ([ordered]@{ name = $name; schema_version = [int]$manifest.schema_version; safety_backup = (Split-Path $safety -Leaf) })
    Write-Ok 'Восстановление завершено. Войдите в систему и проверьте журнал и вложения.'
}

function Invoke-Reset {
    if (-not $Force) {
        Write-Warn 'Будут удалены ВСЕ локальные данные дневника: база, файлы, логи, секреты (резервные копии останутся).'
        if ((Read-Host -Prompt 'Введите RESET для продолжения') -ne 'RESET') { throw 'Отменено пользователем' }
    }
    Invoke-Stop
    foreach ($path in @($PgData, $FilesDir, $LogDir, $RunDir, $ConfigPath)) { Remove-Item -Path $path -Recurse -Force -ErrorAction SilentlyContinue }
    Write-Ok 'Локальные данные удалены. Для нового запуска: diary.cmd setup'
}

function Show-Help {
    @'
Школьный дневник — локальный запуск на Windows (без Docker)

  diary.cmd doctor        проверить, что установлены Go, Node.js и PostgreSQL
  diary.cmd setup         собрать frontend/backend, создать приватный кластер PostgreSQL и базу
  diary.cmd start         запустить PostgreSQL, server и worker
  diary.cmd stop          остановить всё
  diary.cmd restart       перезапустить server и worker
  diary.cmd status        показать состояние и адрес
  diary.cmd logs          последние строки журналов
  diary.cmd create-admin  создать первого системного администратора (пароль запрашивается скрытым вводом)
  diary.cmd seed-demo [-Rich]  загрузить демо-данные (только для тестирования; -Rich — расширенный набор для графиков)
  diary.cmd backup [-Quiesce]            резервная копия в .local\backups (с -Quiesce — согласованная, с остановкой)
  diary.cmd restore -Archive <папка> [-Force]   восстановление из копии
  diary.cmd reset [-Force]               удалить все локальные данные

Подробно: docs\windows-local.md
'@ | Write-Host
}

try {
    switch ($Command) {
        'help' { Show-Help }
        'doctor' { if (-not (Show-Doctor (Get-Tools))) { exit 1 } }
        'setup' { Invoke-Setup }
        'start' { Invoke-Start }
        'stop' { Invoke-Stop }
        'restart' { Stop-DiaryProcess 'worker'; Stop-DiaryProcess 'server'; Invoke-Start }
        'status' { Invoke-Status }
        'logs' { Invoke-Logs }
        'create-admin' { Invoke-CreateAdmin }
        'seed-demo' { Invoke-SeedDemo }
        'backup' { Invoke-Backup }
        'restore' { Invoke-Restore }
        'reset' { Invoke-Reset }
    }
} catch {
    Write-Fail $_.Exception.Message
    exit 1
}
