param(
    [switch]$Dev,
    [string]$Version = $env:VERSION,
    [string]$AppVersion = $env:APP_VERSION,
    [string]$OutputFile = $env:OUTPUT_FILE,
    [string]$IconSource = $env:ICON_SOURCE,
    [string]$AppIconOutputFile = $env:APP_ICON_OUTPUT_FILE,
    [string]$IconSize = $env:ICON_SIZE
)

$ErrorActionPreference = "Stop"

function Require-Command {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        $hint = switch ($Name) {
            "go" { "winget install GoLang.Go" }
            "npm" { "winget install OpenJS.NodeJS.LTS" }
            "magick" { "winget install ImageMagick.ImageMagick" }
            default { "" }
        }

        $message = "Missing required command: $Name"
        if (-not [string]::IsNullOrWhiteSpace($hint)) {
            $message = "$message`nInstall it with: $hint`nThen open a new terminal and rerun this script."
        }
        throw $message
    }
}

$RootDir = Split-Path -Parent $PSScriptRoot

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = "dev"
}
if ([string]::IsNullOrWhiteSpace($AppVersion)) {
    $AppVersion = $Version
}
if ([string]::IsNullOrWhiteSpace($OutputFile)) {
    $OutputFile = Join-Path $RootDir "build/bin/investgo-windows-amd64.exe"
}
if ([string]::IsNullOrWhiteSpace($IconSource)) {
    $IconSource = Join-Path $RootDir "frontend/src/assets/appicon.png"
}
if ([string]::IsNullOrWhiteSpace($AppIconOutputFile)) {
    $AppIconOutputFile = Join-Path $RootDir "build/appicon.png"
}
if ([string]::IsNullOrWhiteSpace($IconSize)) {
    $IconSize = "1024"
}
if ([string]::IsNullOrWhiteSpace($env:GOCACHE)) {
    $env:GOCACHE = Join-Path $env:TEMP "go-build-cache"
}

Require-Command "npm"
Require-Command "go"

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OutputFile) | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $AppIconOutputFile) | Out-Null

Push-Location $RootDir
try {
    if (-not (Test-Path $AppIconOutputFile)) {
        if (-not (Test-Path $IconSource)) {
            throw "Missing icon source file: $IconSource"
        }

        if ([System.IO.Path]::GetExtension($IconSource).ToLowerInvariant() -eq ".png") {
            Copy-Item -Path $IconSource -Destination $AppIconOutputFile -Force
            Write-Host "Copied $AppIconOutputFile"
        } else {
            Require-Command "magick"
            & magick -background none -resize "${IconSize}x${IconSize}" $IconSource $AppIconOutputFile
            if ($LASTEXITCODE -ne 0) {
                throw "Icon rendering failed."
            }
            Write-Host "Rendered $AppIconOutputFile"
        }
    }

    & npm run build
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend build failed."
    }

    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"

    $ldflags = "-s -w -X main.appVersion=$AppVersion"
    $buildTags = "production"
    if ($Dev) {
        $ldflags = "$ldflags -X main.defaultTerminalLogging=1 -X main.defaultDevToolsBuild=1"
        $buildTags = "production devtools"
    }

    & go build -tags $buildTags -trimpath -ldflags $ldflags -o $OutputFile .
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed."
    }

    Write-Host "Built $OutputFile"
}
finally {
    Pop-Location
}
