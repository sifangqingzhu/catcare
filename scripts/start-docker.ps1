$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
& "$PSScriptRoot\init-env.ps1"
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Docker Desktop is required. Install/start Docker Desktop, then rerun this script.' }
& docker compose up -d --build
if ($LASTEXITCODE -ne 0) { throw 'Docker Compose deployment failed.' }
Write-Output 'Catcare is starting. Check docker compose ps; default URL: http://127.0.0.1:8092/'
