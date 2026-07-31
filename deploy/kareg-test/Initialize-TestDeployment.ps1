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
$upgradeExisting = $false
if ((Test-Path -LiteralPath $environmentPath) -and -not $Force) {
    $existingKeys = @{}
    foreach ($line in [System.IO.File]::ReadAllLines($environmentPath)) {
        if ($line -match '^([A-Z0-9_]+)=') {
            $existingKeys[$Matches[1]] = $true
        }
    }

    $compatibilityKeys = @(
        "PORTAL_S3_ACCESS_KEY",
        "PORTAL_S3_SECRET_KEY",
        "SYNAPSE_S3_ACCESS_KEY",
        "SYNAPSE_S3_SECRET_KEY",
        "BACKUP_S3_ACCESS_KEY",
        "BACKUP_S3_SECRET_KEY"
    )
    $missingCompatibilityKeys = @($compatibilityKeys | Where-Object { -not $existingKeys.ContainsKey($_) })
    if ($missingCompatibilityKeys.Count -eq 0) {
        Write-Host "Existing deployment environment retained: $environmentPath"
        exit 0
    }
    if ($missingCompatibilityKeys.Count -ne $compatibilityKeys.Count) {
        throw "The existing deployment environment has a partial compatibility-key set. Run with -Force to replace it consistently."
    }
    $upgradeExisting = $true
}

New-Item -ItemType Directory -Path $resolvedDataPath -Force | Out-Null

function New-HexValue([int]$ByteCount) {
    $random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $bytes = New-Object byte[] $ByteCount
        $random.GetBytes($bytes)
        return -join ($bytes | ForEach-Object { $_.ToString("x2") })
    }
    finally {
        $random.Dispose()
    }
}

$password = New-HexValue 32
$composeDataPath = $resolvedDataPath.Replace("\", "/")
$baseLines = @(
    "SILO_IMAGE=ghcr.io/karnetk/silo:RELEASE.2026-07-31T00-17-54Z"
    "SILO_ROOT_USER=silo-test-admin"
    "SILO_ROOT_PASSWORD=$password"
    "SILO_DATA_PATH=$composeDataPath"
    "SILO_API_PORT=19000"
    "SILO_CONSOLE_PORT=19001"
)
$compatibilityLines = @(
    "PORTAL_S3_ACCESS_KEY=portal-$(New-HexValue 6)"
    "PORTAL_S3_SECRET_KEY=$(New-HexValue 20)"
    "SYNAPSE_S3_ACCESS_KEY=synapse-$(New-HexValue 6)"
    "SYNAPSE_S3_SECRET_KEY=$(New-HexValue 20)"
    "BACKUP_S3_ACCESS_KEY=backup-$(New-HexValue 6)"
    "BACKUP_S3_SECRET_KEY=$(New-HexValue 20)"
)

$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
if ($upgradeExisting) {
    $writer = New-Object System.IO.StreamWriter($environmentPath, $true, $utf8WithoutBom)
    try {
        foreach ($line in $compatibilityLines) {
            $writer.WriteLine($line)
        }
    }
    finally {
        $writer.Dispose()
    }
    Write-Host "Portal compatibility credentials added: $environmentPath"
}
else {
    [System.IO.File]::WriteAllLines($environmentPath, $baseLines + $compatibilityLines, $utf8WithoutBom)
    Write-Host "Deployment environment created: $environmentPath"
}
Write-Host "Persistent data directory: $resolvedDataPath"
