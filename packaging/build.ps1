[CmdletBinding()]
param(
    [Parameter(Position = 0, ValueFromRemainingArguments = $true)]
    [string[]]$Target = @('all'),
    [switch]$Clean,
    [switch]$SkipTest,
    [switch]$List
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$distDir = Join-Path $PSScriptRoot 'dist'
$buildFlags = @('-ldflags', '-s -w', '-trimpath')

$targets = @(
    [pscustomobject]@{ Name = 'windows-amd64'; OS = 'windows'; Arch = 'amd64'; Arm = ''; AMD64 = 'v1'; Output = 'go2rtc_win64.exe' }
    [pscustomobject]@{ Name = 'windows-386'; OS = 'windows'; Arch = '386'; Arm = ''; Output = 'go2rtc_win32.exe' }
    [pscustomobject]@{ Name = 'windows-arm64'; OS = 'windows'; Arch = 'arm64'; Arm = ''; Output = 'go2rtc_win_arm64.exe' }
    [pscustomobject]@{ Name = 'linux-amd64'; OS = 'linux'; Arch = 'amd64'; Arm = ''; AMD64 = 'v1'; Output = 'go2rtc_linux_amd64' }
    [pscustomobject]@{ Name = 'linux-386'; OS = 'linux'; Arch = '386'; Arm = ''; Output = 'go2rtc_linux_i386' }
    [pscustomobject]@{ Name = 'linux-arm64'; OS = 'linux'; Arch = 'arm64'; Arm = ''; Output = 'go2rtc_linux_arm64' }
    [pscustomobject]@{ Name = 'linux-armv7'; OS = 'linux'; Arch = 'arm'; Arm = '7'; Output = 'go2rtc_linux_arm' }
    [pscustomobject]@{ Name = 'linux-armv6'; OS = 'linux'; Arch = 'arm'; Arm = '6'; Output = 'go2rtc_linux_armv6' }
    [pscustomobject]@{ Name = 'linux-mipsle'; OS = 'linux'; Arch = 'mipsle'; Arm = ''; Output = 'go2rtc_linux_mipsel' }
    [pscustomobject]@{ Name = 'darwin-amd64'; OS = 'darwin'; Arch = 'amd64'; Arm = ''; AMD64 = 'v1'; Output = 'go2rtc_mac_amd64' }
    [pscustomobject]@{ Name = 'darwin-arm64'; OS = 'darwin'; Arch = 'arm64'; Arm = ''; Output = 'go2rtc_mac_arm64' }
    [pscustomobject]@{ Name = 'freebsd-amd64'; OS = 'freebsd'; Arch = 'amd64'; Arm = ''; AMD64 = 'v1'; Output = 'go2rtc_freebsd_amd64' }
    [pscustomobject]@{ Name = 'freebsd-arm64'; OS = 'freebsd'; Arch = 'arm64'; Arm = ''; Output = 'go2rtc_freebsd_arm64' }
)

function Show-Targets {
    Write-Host 'Available targets:'
    $targets | Format-Table Name, OS, Arch, Arm, Output -AutoSize
    Write-Host 'Groups: all, windows, linux, darwin (aliases: mac, macos), freebsd'
}

function Resolve-Targets {
    param([string[]]$Selectors)

    $resolved = [System.Collections.Generic.List[object]]::new()
    $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)

    foreach ($rawSelector in $Selectors) {
        foreach ($selectorValue in $rawSelector.Split(',', [System.StringSplitOptions]::RemoveEmptyEntries)) {
            $selector = $selectorValue.Trim().ToLowerInvariant()
            $matches = switch ($selector) {
                'all' { $targets; break }
                'windows' { $targets | Where-Object OS -eq 'windows'; break }
                'linux' { $targets | Where-Object OS -eq 'linux'; break }
                { $_ -in @('darwin', 'mac', 'macos') } { $targets | Where-Object OS -eq 'darwin'; break }
                'freebsd' { $targets | Where-Object OS -eq 'freebsd'; break }
                default { $targets | Where-Object Name -eq $selector }
            }

            if (-not $matches) {
                throw "Unknown build target: $selectorValue. Use -List to show available targets."
            }
            foreach ($item in @($matches)) {
                if ($seen.Add($item.Name)) {
                    $resolved.Add($item)
                }
            }
        }
    }
    return $resolved.ToArray()
}

function Restore-ProcessEnvironment {
    param([hashtable]$Values)
    foreach ($name in $Values.Keys) {
        if ($null -eq $Values[$name]) {
            Remove-Item "Env:$name" -ErrorAction SilentlyContinue
        } else {
            Set-Item "Env:$name" $Values[$name]
        }
    }
}

function Test-BuildArtifact {
    param(
        [Parameter(Mandatory = $true)]$Item,
        [Parameter(Mandatory = $true)][string]$Path
    )

    $file = Get-Item -LiteralPath $Path -ErrorAction Stop
    if ($file.Length -le 0) {
        throw "Build produced an empty artifact: $Path"
    }

    $metadata = (& go version -m $Path 2>&1 | Out-String)
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to inspect build artifact: $Path`n$metadata"
    }
    foreach ($expected in @("GOOS=$($Item.OS)", "GOARCH=$($Item.Arch)", 'CGO_ENABLED=0')) {
        if ($metadata -notmatch [regex]::Escape($expected)) {
            throw "Artifact metadata is missing $expected`: $Path"
        }
    }
    if ($Item.Arch -eq 'amd64' -and $metadata -notmatch [regex]::Escape("GOAMD64=$($Item.AMD64)")) {
        throw "Artifact metadata is missing GOAMD64=$($Item.AMD64)`: $Path"
    }
}

if ($List) {
    Show-Targets
    exit 0
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go was not found. Install Go 1.25 or newer and add it to PATH.'
}

$selectedTargets = @(Resolve-Targets $Target)
New-Item -ItemType Directory -Path $distDir -Force | Out-Null

if ($Clean) {
    $scriptRootPath = [System.IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\', '/')
    $distPath = [System.IO.Path]::GetFullPath($distDir).TrimEnd('\', '/')
    if ([System.IO.Path]::GetDirectoryName($distPath) -ne $scriptRootPath -or [System.IO.Path]::GetFileName($distPath) -ne 'dist') {
        throw "Refusing to clean unexpected directory: $distPath"
    }
    Get-ChildItem -LiteralPath $distPath -Force | Remove-Item -Recurse -Force
}

$originalEnvironment = @{}
foreach ($name in @('CGO_ENABLED', 'GOOS', 'GOARCH', 'GOARM', 'GOAMD64')) {
    $originalEnvironment[$name] = [System.Environment]::GetEnvironmentVariable($name, 'Process')
}

$locationPushed = $false
try {
    Push-Location $repoRoot
    $locationPushed = $true

    Write-Host '==> Validating VERSION' -ForegroundColor Cyan
    & go run ./cmd/version check
    if ($LASTEXITCODE -ne 0) {
        throw 'VERSION validation failed.'
    }

    if (-not $SkipTest) {
        Write-Host '==> Running pre-build test: go test -count=1 ./internal/api' -ForegroundColor Cyan
        & go test -count=1 ./internal/api
        if ($LASTEXITCODE -ne 0) {
            throw 'Pre-build test failed.'
        }
    }

    $index = 0
    foreach ($item in $selectedTargets) {
        $index++
        $outputPath = Join-Path $distDir $item.Output
        Write-Host "==> [$index/$($selectedTargets.Count)] $($item.Name) -> $($item.Output)" -ForegroundColor Cyan

        $env:CGO_ENABLED = '0'
        $env:GOOS = $item.OS
        $env:GOARCH = $item.Arch
        if ($item.Arm) {
            $env:GOARM = $item.Arm
        } else {
            Remove-Item Env:GOARM -ErrorAction SilentlyContinue
        }
        if ($item.Arch -eq 'amd64') {
            $env:GOAMD64 = $item.AMD64
        } else {
            Remove-Item Env:GOAMD64 -ErrorAction SilentlyContinue
        }

        & go build @buildFlags -o $outputPath .
        if ($LASTEXITCODE -ne 0) {
            throw "Build failed: $($item.Name)"
        }
        Test-BuildArtifact -Item $item -Path $outputPath
    }
} finally {
    if ($locationPushed) {
        Pop-Location
    }
    Restore-ProcessEnvironment $originalEnvironment
}

$checksumPath = Join-Path $distDir 'SHA256SUMS.txt'
$artifactFiles = Get-ChildItem -LiteralPath $distDir -File -Force |
    Where-Object Name -NotIn @('SHA256SUMS.txt', '.gitignore') |
    Sort-Object Name
$checksumLines = foreach ($file in $artifactFiles) {
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $($file.Name)"
}
[System.IO.File]::WriteAllLines($checksumPath, $checksumLines, [System.Text.UTF8Encoding]::new($false))

Write-Host "`nBuild completed: $distDir" -ForegroundColor Green
Write-Host "SHA256 checksums: $checksumPath" -ForegroundColor Green
