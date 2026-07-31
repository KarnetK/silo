[CmdletBinding()]
param(
    [string]$DataPath = "E:\SiloTest\data",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

$resolvedDataPath = [System.IO.Path]::GetFullPath($DataPath)
if ([System.IO.Path]::GetPathRoot($resolvedDataPath) -ne "E:\") {
    throw "Persistent Silo TEST data must be stored on drive E:."
}

$environmentPath = Join-Path $PSScriptRoot ".env"
if ((Test-Path -LiteralPath $environmentPath) -and -not $Force) {
    Write-Host "Existing deployment environment retained: $environmentPath"
    exit 0
}

New-Item -ItemType Directory -Path $resolvedDataPath -Force | Out-Null

$random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $passwordBytes = New-Object byte[] 32
    $random.GetBytes($passwordBytes)
}
finally {
    $random.Dispose()
}

$password = -join ($passwordBytes | ForEach-Object { $_.ToString("x2") })
$composeDataPath = $resolvedDataPath.Replace("\", "/")
$lines = @(
    "SILO_IMAGE=ghcr.io/karnetk/silo:RELEASE.2026-07-31T00-17-54Z"
    "SILO_ROOT_USER=silo-test-admin"
    "SILO_ROOT_PASSWORD=$password"
    "SILO_DATA_PATH=$composeDataPath"
    "SILO_API_PORT=19000"
    "SILO_CONSOLE_PORT=19001"
)

$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllLines($environmentPath, $lines, $utf8WithoutBom)
Write-Host "Deployment environment created: $environmentPath"
Write-Host "Persistent data directory: $resolvedDataPath"
