param(
    [Parameter(Mandatory = $true)]
    [string]$ModuleDir,

    [Parameter(Mandatory = $false)]
    [string]$OutDir = "artifacts/extensions",

    [Parameter(Mandatory = $false)]
    [switch]$KeepTemp
)

$ErrorActionPreference = "Stop"

function Test-ExcludedPath {
    param(
        [Parameter(Mandatory = $true)][string]$RelativePath
    )

    $path = $RelativePath.Replace('\', '/').Trim('/')
    if ([string]::IsNullOrWhiteSpace($path)) { return $false }

    $segments = $path.Split('/')
    $excludedDirs = @(
        ".git", ".idea", ".vscode", ".cache", ".turbo", ".next", ".vite",
        "node_modules", "dist", "build", "coverage", ".nyc_output", "logs",
        "tmp", "temp", "__pycache__"
    )
    foreach ($segment in $segments) {
        if ($excludedDirs -contains $segment) { return $true }
    }

    $fileName = [System.IO.Path]::GetFileName($path)
    $excludedPatterns = @(
        "*.zip", "*.tpm", "*.tar", "*.tar.gz", "*.7z", "*.rar",
        "*.log", "*.tmp", "*.db", "*.db-shm", "*.db-wal", "*.sqlite",
        "*.sqlite3", "*.pem", "*.key", "*.pfx", "*.p12", ".env", ".env.*",
        "secrets.json", "secret.json", ".DS_Store", "Thumbs.db"
    )
    foreach ($pattern in $excludedPatterns) {
        if ($fileName -like $pattern) { return $true }
    }

    return $false
}

function Get-RelativePath {
    param(
        [Parameter(Mandatory = $true)][string]$Base,
        [Parameter(Mandatory = $true)][string]$Target
    )

    $basePath = (Resolve-Path $Base).Path
    $targetPath = (Resolve-Path $Target).Path

    if ([System.IO.Path].GetMethods().Name -contains "GetRelativePath") {
        return [System.IO.Path]::GetRelativePath($basePath, $targetPath).Replace('\', '/')
    }

    $baseUri = New-Object System.Uri(($basePath.TrimEnd('\') + '\'))
    $targetUri = New-Object System.Uri($targetPath)
    return [System.Uri]::UnescapeDataString($baseUri.MakeRelativeUri($targetUri).ToString()).Replace('\', '/')
}

$repoRoot = (Resolve-Path ".").Path
$modulePath = (Resolve-Path $ModuleDir).Path
$manifestPath = Join-Path $modulePath "manifest.json"
if (-not (Test-Path -Path $manifestPath)) {
    throw "manifest.json 不存在：$modulePath"
}

$manifest = Get-Content -Path $manifestPath -Raw | ConvertFrom-Json
if ($null -eq $manifest.id -or [string]::IsNullOrWhiteSpace([string]$manifest.id)) {
    throw "manifest.json 缺少 id"
}
if ($null -eq $manifest.name -or [string]::IsNullOrWhiteSpace([string]$manifest.name)) {
    throw "manifest.json 缺少 name"
}
if ($null -eq $manifest.version -or [string]::IsNullOrWhiteSpace([string]$manifest.version)) {
    throw "manifest.json 缺少 version"
}
if ($null -eq $manifest.runtime -or [string]::IsNullOrWhiteSpace([string]$manifest.runtime.base_url)) {
    throw "manifest.json 缺少 runtime.base_url"
}

$moduleId = [string]$manifest.id
$moduleVersion = [string]$manifest.version
if ($moduleId -notmatch '^[A-Za-z0-9_-]+$') {
    throw "manifest.id 只能包含字母、数字、短横线和下划线：$moduleId"
}

$outPath = if ([System.IO.Path]::IsPathRooted($OutDir)) { $OutDir } else { Join-Path $repoRoot $OutDir }
New-Item -ItemType Directory -Path $outPath -Force | Out-Null

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("newapi-extension-{0}-{1}" -f $moduleId, [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

try {
    $files = Get-ChildItem -Path $modulePath -Recurse -File | Where-Object {
        $relative = Get-RelativePath -Base $modulePath -Target $_.FullName
        -not (Test-ExcludedPath -RelativePath $relative)
    }

    if ($files.Count -eq 0) {
        throw "模块目录没有可打包文件：$modulePath"
    }

    foreach ($file in $files) {
        $relative = Get-RelativePath -Base $modulePath -Target $file.FullName
        $target = Join-Path $tempRoot $relative
        New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force | Out-Null
        Copy-Item -Path $file.FullName -Destination $target -Force
    }

    $packedManifest = Join-Path $tempRoot "manifest.json"
    if (-not (Test-Path -Path $packedManifest)) {
        throw "打包结果缺少 manifest.json"
    }

    $archivePath = Join-Path $outPath ("{0}-{1}.zip" -f $moduleId, $moduleVersion)
    if (Test-Path -Path $archivePath) {
        Remove-Item -LiteralPath $archivePath -Force
    }

    Compress-Archive -Path (Join-Path $tempRoot "*") -DestinationPath $archivePath -CompressionLevel Optimal

    $archive = Get-Item -Path $archivePath
    $sizeKb = [Math]::Round($archive.Length / 1KB, 2)
    Write-Host ("产物: {0}" -f $archive.FullName) -ForegroundColor Green
    Write-Host ("大小: {0} KB" -f $sizeKb) -ForegroundColor Green
    if ($archive.Length -gt 1MB) {
        Write-Warning "模块包超过 1 MiB，请检查是否误带依赖、构建产物、数据库或日志。"
    }
} finally {
    if (-not $KeepTemp -and (Test-Path -Path $tempRoot)) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    } elseif ($KeepTemp) {
        Write-Host ("临时目录: {0}" -f $tempRoot) -ForegroundColor Yellow
    }
}
