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
        "tmp", "temp", "__pycache__", "native-src"
    )
    foreach ($segment in $segments) {
        if ($excludedDirs -contains $segment) { return $true }
    }

    $fileName = [System.IO.Path]::GetFileName($path)
    $excludedPatterns = @(
        "*.zip", "*.tpm", "*.tar", "*.tar.gz", "*.7z", "*.rar",
        "*.log", "*.tmp", "*.db", "*.db-shm", "*.db-wal", "*.sqlite",
        "*.sqlite3", "*.pem", "*.key", "*.pfx", "*.p12", ".env", ".env.*",
        "secrets.json", "secret.json", ".extensionignore", ".DS_Store", "Thumbs.db"
    )
    foreach ($pattern in $excludedPatterns) {
        if ($fileName -like $pattern) { return $true }
    }

    return $false
}

function Test-StaticRuntimeExcludedPath {
    param(
        [Parameter(Mandatory = $true)][string]$RelativePath,
        [Parameter(Mandatory = $true)][string]$StaticDir
    )

    $path = $RelativePath.Replace('\', '/').Trim('/')
    if ([string]::IsNullOrWhiteSpace($path)) { return $false }

    # static 模块的静态资源目录可以合法包含 app.js 等浏览器脚本。
    # 静态目录之外仍需排除同名服务端入口，避免将后端代码误打进模块包。
    $normalizedStaticDir = $StaticDir.Replace('\', '/').Trim('/')
    if ($path.StartsWith($normalizedStaticDir + '/', [System.StringComparison]::OrdinalIgnoreCase)) {
        return $false
    }

    $fileName = [System.IO.Path]::GetFileName($path)
    $staticRuntimeServerEntries = @(
        "server.mjs", "server.js", "server.cjs",
        "app.mjs", "app.js", "app.cjs"
    )
    return $staticRuntimeServerEntries -contains $fileName
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

function Assert-NativeResources {
    param(
        [Parameter(Mandatory = $true)][object]$Manifest,
        [Parameter(Mandatory = $true)][string]$PackageRoot,
        [Parameter(Mandatory = $true)][string]$StaticDir
    )

    $pages = @($Manifest.ui.pages)
    foreach ($page in $pages) {
        $render = $page.render
        if ($null -eq $render -or [string]$render.type -ne "native") { continue }
        if ([string]$Manifest.runtime.type -ne "static") {
            throw "原生页面只能由 static 模块提供：$($page.key)"
        }

        $targets = $render.targets
        if ($null -eq $targets) {
            throw "原生页面缺少 render.targets：$($page.key)"
        }

        foreach ($targetName in @("default", "classic")) {
            $target = $targets.$targetName
            if ($null -eq $target) {
                throw "原生页面缺少 $targetName 目标：$($page.key)"
            }

            $resources = @([string]$target.entry)
            if ($null -ne $target.styles) {
                $resources += @($target.styles | ForEach-Object { [string]$_ })
            }

            foreach ($resource in $resources) {
                $candidate = $resource.Replace('\', '/')
                $normalized = $candidate.Trim('/')
                if ([string]::IsNullOrWhiteSpace($normalized) -or
                    $candidate.StartsWith("//") -or
                    $candidate -match '^[A-Za-z]:' -or
                    $normalized -eq ".." -or
                    $normalized.StartsWith("../") -or
                    $normalized.Contains("/../")) {
                    throw "原生资源路径不安全：$resource"
                }

                $resourcePath = Join-Path (Join-Path $PackageRoot $StaticDir) $normalized
                if (-not (Test-Path -LiteralPath $resourcePath -PathType Leaf)) {
                    throw "打包结果缺少原生资源：$resource"
                }
            }
        }
    }
}

$repoRoot = (Resolve-Path ".").Path
$modulePath = (Resolve-Path $ModuleDir).Path
$manifestPath = Join-Path $modulePath "manifest.json"
if (-not (Test-Path -Path $manifestPath)) {
    throw "manifest.json 不存在：$modulePath"
}

$manifest = Get-Content -Path $manifestPath -Raw | ConvertFrom-Json
$moduleIgnoreEntries = @()
$moduleIgnorePath = Join-Path $modulePath ".extensionignore"
if (Test-Path -LiteralPath $moduleIgnorePath -PathType Leaf) {
    foreach ($line in Get-Content -LiteralPath $moduleIgnorePath) {
        $entry = $line.Trim().Replace('\', '/').Trim('/')
        if ([string]::IsNullOrWhiteSpace($entry) -or $entry.StartsWith('#')) { continue }
        if ([System.IO.Path]::IsPathRooted($entry) -or $entry -eq ".." -or $entry.StartsWith("../") -or $entry.Contains("/../")) {
            throw ".extensionignore 包含不安全路径：$line"
        }
        $moduleIgnoreEntries += $entry
    }
}
if ($null -eq $manifest.id -or [string]::IsNullOrWhiteSpace([string]$manifest.id)) {
    throw "manifest.json 缺少 id"
}
if ($null -eq $manifest.name -or [string]::IsNullOrWhiteSpace([string]$manifest.name)) {
    throw "manifest.json 缺少 name"
}
if ($null -eq $manifest.version -or [string]::IsNullOrWhiteSpace([string]$manifest.version)) {
    throw "manifest.json 缺少 version"
}
if ($null -eq $manifest.runtime) {
    throw "manifest.json 缺少 runtime"
}
$runtimeType = [string]$manifest.runtime.type
if ([string]::IsNullOrWhiteSpace($runtimeType)) {
    $runtimeType = "http"
}
$normalizedStaticDir = ""
if ($runtimeType -eq "http" -and [string]::IsNullOrWhiteSpace([string]$manifest.runtime.base_url)) {
    throw "manifest.json 缺少 runtime.base_url"
}
if ($runtimeType -eq "static") {
    $staticDir = [string]$manifest.runtime.static_dir
    if ([string]::IsNullOrWhiteSpace($staticDir)) {
        $staticDir = "public"
    }
    $normalizedStaticDir = $staticDir.Replace('\', '/').Trim('/')
    if ([string]::IsNullOrWhiteSpace($normalizedStaticDir) -or
        [System.IO.Path]::IsPathRooted($staticDir) -or
        $normalizedStaticDir -eq "." -or
        $normalizedStaticDir -eq ".." -or
        $normalizedStaticDir.StartsWith("../") -or
        $normalizedStaticDir.Contains("/../")) {
        throw "manifest.json 指定的 runtime.static_dir 不安全：$staticDir"
    }
    $staticPath = Join-Path $modulePath $staticDir
    if (-not (Test-Path -Path $staticPath -PathType Container)) {
        throw "manifest.json 指定的 runtime.static_dir 不存在：$staticDir"
    }
} elseif ($runtimeType -ne "http") {
    throw "manifest.json runtime.type 仅支持 http 或 static：$runtimeType"
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
        -not (Test-ExcludedPath -RelativePath $relative) -and
        -not ($moduleIgnoreEntries -contains $relative.Replace('\', '/').Trim('/')) -and
        -not ($runtimeType -eq "static" -and (Test-StaticRuntimeExcludedPath -RelativePath $relative -StaticDir $normalizedStaticDir))
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
    Assert-NativeResources -Manifest $manifest -PackageRoot $tempRoot -StaticDir $normalizedStaticDir

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
