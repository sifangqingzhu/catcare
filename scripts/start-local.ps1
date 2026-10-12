$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
& "$PSScriptRoot\local-db.ps1"
Set-Location (Join-Path $taskRoot 'backend')
if (-not (Test-Path -LiteralPath '.\catcare.exe')) { throw 'Build backend/catcare.exe first; Docker Compose is the recommended deployment path.' }
& '.\catcare.exe'
