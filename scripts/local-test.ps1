param(
    [ValidateSet('status', 'start', 'stop', 'restart', 'verify')]
    [string]$Action = 'status',

    [switch]$BuildBackend,
    [switch]$SkipBackendBuild,
    [switch]$NoVerify
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir
$BackendPort = 3000
$FrontendPort = 3001
$DbPath = Join-Path $RepoRoot 'tmp-local-v10101.db'
$LogDir = Join-Path $RepoRoot 'tmp-local-logs'
$ClassicDir = Join-Path $RepoRoot 'web\classic'
$BackendLog = Join-Path $LogDir 'backend.out.log'
$BackendErrLog = Join-Path $LogDir 'backend.err.log'
$FrontendLog = Join-Path $LogDir 'classic.out.log'
$FrontendErrLog = Join-Path $LogDir 'classic.err.log'

function Write-Info {
    param([string]$Message)
    Write-Host "[local-test] $Message"
}

function Test-Contains {
    param(
        [AllowNull()][string]$Text,
        [string]$Needle
    )

    if ([string]::IsNullOrWhiteSpace($Text)) {
        return $false
    }

    return $Text.IndexOf($Needle, [StringComparison]::OrdinalIgnoreCase) -ge 0
}

function Test-PathInsideRepo {
    param([AllowNull()][string]$Path)

    if ([string]::IsNullOrWhiteSpace($Path)) {
        return $false
    }

    try {
        $fullPath = [System.IO.Path]::GetFullPath($Path)
        $fullRoot = [System.IO.Path]::GetFullPath($RepoRoot)
        return $fullPath.StartsWith($fullRoot, [StringComparison]::OrdinalIgnoreCase)
    }
    catch {
        return $false
    }
}

function Get-ListeningProcesses {
    param([int]$Port)

    $connections = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue
    if (-not $connections) {
        return @()
    }

    $processIds = @($connections | Select-Object -ExpandProperty OwningProcess -Unique)
    $items = foreach ($processId in $processIds) {
        Get-CimInstance Win32_Process -Filter "ProcessId=$processId" -ErrorAction SilentlyContinue
    }

    return @($items | Where-Object { $null -ne $_ })
}

function Test-LocalTestProcess {
    param(
        [Parameter(Mandatory = $true)]$Process,
        [ValidateSet('backend', 'frontend')]
        [string]$Kind
    )

    $commandLine = [string]$Process.CommandLine
    $exePath = [string]$Process.ExecutablePath
    $inRepo = (Test-PathInsideRepo $exePath) -or (Test-Contains $commandLine $RepoRoot)

    if ($Kind -eq 'backend') {
        return $inRepo -and (
            (Test-Contains $exePath 'tmp-local-newapi') -or
            (Test-Contains $commandLine 'tmp-local-newapi') -or
            ((Test-Contains $commandLine 'go run') -and (Test-Contains $commandLine 'main.go'))
        )
    }

    return $inRepo -and
        (Test-Contains $commandLine $ClassicDir) -and
        (Test-Contains $commandLine 'vite') -and
        (Test-Contains $commandLine "--port $FrontendPort")
}

function Stop-PortProcess {
    param(
        [int]$Port,
        [ValidateSet('backend', 'frontend')]
        [string]$Kind
    )

    $processes = Get-ListeningProcesses -Port $Port
    foreach ($process in $processes) {
        if (-not (Test-LocalTestProcess -Process $process -Kind $Kind)) {
            throw "端口 $Port 被非本项目本地测试进程占用，PID=$($process.ProcessId)，不会误杀。命令行：$($process.CommandLine)"
        }

        Write-Info "停止旧 $Kind 测试进程：PID=$($process.ProcessId)，端口=$Port"
        Stop-Process -Id $process.ProcessId -Force
    }
}

function Wait-PortFree {
    param([int]$Port)

    for ($i = 0; $i -lt 40; $i++) {
        if (-not (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue)) {
            return
        }
        Start-Sleep -Milliseconds 250
    }

    throw "端口 $Port 在等待后仍未释放"
}

function Wait-PortListen {
    param([int]$Port)

    for ($i = 0; $i -lt 80; $i++) {
        if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) {
            return
        }
        Start-Sleep -Milliseconds 250
    }

    throw "端口 $Port 未在预期时间内开始监听"
}

function Get-BackendExecutable {
    if ($BuildBackend -or (-not $SkipBackendBuild)) {
        $target = Join-Path $RepoRoot 'tmp-local-newapi-dev.exe'
        Write-Info "构建本地后端二进制：$target"
        Push-Location $RepoRoot
        try {
            & go build -o $target .
            if ($LASTEXITCODE -ne 0) {
                throw "go build 失败，退出码 $LASTEXITCODE"
            }
        }
        finally {
            Pop-Location
        }
        return $target
    }

    $existing = Get-ChildItem -Path $RepoRoot -Filter 'tmp-local-newapi*.exe' -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1

    if ($existing) {
        return $existing.FullName
    }

    throw "没有找到 tmp-local-newapi*.exe。需要重新构建时请加 -BuildBackend。"
}

function Start-Backend {
    if (-not (Test-Path $DbPath)) {
        throw "固定测试数据库不存在：$DbPath"
    }

    if (-not (Test-Path $LogDir)) {
        New-Item -ItemType Directory -Path $LogDir | Out-Null
    }

    $backendExe = Get-BackendExecutable
    $env:PORT = [string]$BackendPort
    $env:SQLITE_PATH = "$DbPath`?_busy_timeout=30000"
    $env:SESSION_COOKIE_SECURE = 'false'

    Write-Info "启动后端：端口=$BackendPort，数据库=tmp-local-v10101.db"
    $process = Start-Process `
        -FilePath $backendExe `
        -ArgumentList @('--log-dir', $LogDir) `
        -WorkingDirectory $RepoRoot `
        -WindowStyle Hidden `
        -RedirectStandardOutput $BackendLog `
        -RedirectStandardError $BackendErrLog `
        -PassThru

    Write-Info "后端 PID=$($process.Id)"
    Wait-PortListen -Port $BackendPort
}

function Start-Frontend {
    if (-not (Test-Path $LogDir)) {
        New-Item -ItemType Directory -Path $LogDir | Out-Null
    }

    $bun = Get-Command bun -ErrorAction SilentlyContinue
    if ($bun) {
        $filePath = $bun.Source
        $argumentList = @('run', 'dev', '--', '--host', '0.0.0.0', '--port', [string]$FrontendPort, '--force')
    }
    else {
        $node = Get-Command node -ErrorAction Stop
        $vite = Join-Path $ClassicDir 'node_modules\vite\bin\vite.js'
        if (-not (Test-Path $vite)) {
            throw "未找到 classic 前端依赖：$vite"
        }
        $filePath = $node.Source
        $argumentList = @($vite, '--host', '0.0.0.0', '--port', [string]$FrontendPort, '--force')
    }

    Write-Info "启动 classic 前端：端口=$FrontendPort"
    $process = Start-Process `
        -FilePath $filePath `
        -ArgumentList $argumentList `
        -WorkingDirectory $ClassicDir `
        -WindowStyle Hidden `
        -RedirectStandardOutput $FrontendLog `
        -RedirectStandardError $FrontendErrLog `
        -PassThru

    Write-Info "classic 前端 PID=$($process.Id)"
    Wait-PortListen -Port $FrontendPort
}

function Stop-LocalStack {
    Stop-PortProcess -Port $FrontendPort -Kind frontend
    Stop-PortProcess -Port $BackendPort -Kind backend
    Wait-PortFree -Port $FrontendPort
    Wait-PortFree -Port $BackendPort
}

function Start-LocalStack {
    Stop-LocalStack
    Start-Backend
    Start-Frontend

    if (-not $NoVerify) {
        Invoke-LocalVerification
    }
}

function Get-ResponseItemsCount {
    param([AllowNull()]$Response)

    if ($null -eq $Response) {
        return 0
    }

    if ($Response.data -and $Response.data.items) {
        return @($Response.data.items).Count
    }

    if ($Response.data -is [array]) {
        return @($Response.data).Count
    }

    if ($Response.data) {
        return 1
    }

    return 0
}

function Invoke-LocalVerification {
    Write-Info '验证后端 /api/status'
    $status = Invoke-RestMethod -Uri "http://localhost:$BackendPort/api/status" -TimeoutSec 10
    if ($null -eq $status) {
        throw '后端 /api/status 无响应'
    }

    Write-Info '验证前端首页'
    $homeResponse = Invoke-WebRequest -Uri "http://localhost:$FrontendPort/" -UseBasicParsing -TimeoutSec 10
    if ($homeResponse.StatusCode -lt 200 -or $homeResponse.StatusCode -ge 400) {
        throw "前端首页状态码异常：$($homeResponse.StatusCode)"
    }

    Write-Info '验证默认账号 root / 123456'
    $session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $body = @{ username = 'root'; password = '123456' } | ConvertTo-Json
    $login = Invoke-RestMethod `
        -WebSession $session `
        -Uri "http://localhost:$FrontendPort/api/user/login?turnstile=" `
        -Method POST `
        -Body $body `
        -ContentType 'application/json' `
        -TimeoutSec 10

    if (-not $login.success) {
        throw "root 登录失败：$($login.message)"
    }

    $headers = @{ 'New-Api-User' = [string]$login.data.id }
    $channels = Invoke-RestMethod -WebSession $session -Headers $headers -Uri "http://localhost:$FrontendPort/api/channel/?p=1&page_size=10" -TimeoutSec 10
    $models = Invoke-RestMethod -WebSession $session -Headers $headers -Uri "http://localhost:$FrontendPort/api/models/?p=1&page_size=10" -TimeoutSec 10
    $tokens = Invoke-RestMethod -WebSession $session -Headers $headers -Uri "http://localhost:$FrontendPort/api/token/?p=1&page_size=10" -TimeoutSec 10
    $plans = Invoke-RestMethod -WebSession $session -Headers $headers -Uri "http://localhost:$FrontendPort/api/subscription/plans" -TimeoutSec 10

    Write-Info '验证默认账号 demo / 123456'
    $demoSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $demoBody = @{ username = 'demo'; password = '123456' } | ConvertTo-Json
    $demoLogin = Invoke-RestMethod `
        -WebSession $demoSession `
        -Uri "http://localhost:$FrontendPort/api/user/login?turnstile=" `
        -Method POST `
        -Body $demoBody `
        -ContentType 'application/json' `
        -TimeoutSec 10

    if (-not $demoLogin.success) {
        throw "demo 登录失败：$($demoLogin.message)"
    }

    Write-Info "验证通过：渠道=$(Get-ResponseItemsCount $channels)，模型=$(Get-ResponseItemsCount $models)，令牌=$(Get-ResponseItemsCount $tokens)，套餐=$(Get-ResponseItemsCount $plans)"
}

function Show-Status {
    Write-Host ''
    Write-Host '本地测试固定环境'
    Write-Host "  后端端口：$BackendPort"
    Write-Host "  前端端口：$FrontendPort"
    Write-Host "  固定数据库：$DbPath"
    Write-Host "  数据库存在：$(Test-Path $DbPath)"

    foreach ($entry in @(
        @{ Port = $BackendPort; Kind = 'backend' },
        @{ Port = $FrontendPort; Kind = 'frontend' }
    )) {
        $processes = Get-ListeningProcesses -Port $entry.Port
        if (-not $processes) {
            Write-Host "  端口 $($entry.Port)：未监听"
            continue
        }

        foreach ($process in $processes) {
            $safe = Test-LocalTestProcess -Process $process -Kind $entry.Kind
            Write-Host "  端口 $($entry.Port)：PID=$($process.ProcessId)，进程=$($process.Name)，本地测试进程=$safe"
            Write-Host "    $($process.CommandLine)"
        }
    }
}

switch ($Action) {
    'status' {
        Show-Status
    }
    'stop' {
        Stop-LocalStack
        Show-Status
    }
    'start' {
        Start-LocalStack
        Show-Status
    }
    'restart' {
        Start-LocalStack
        Show-Status
    }
    'verify' {
        Invoke-LocalVerification
        Show-Status
    }
}
