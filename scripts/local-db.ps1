# Optional local QA helper. Docker Compose is the supported deployment path.
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Set-Location $taskRoot
$pgSource = Join-Path $taskRoot '.tools\pgsql'
if (-not (Test-Path -LiteralPath (Join-Path $pgSource 'bin\initdb.exe'))) { throw 'Portable PostgreSQL is not installed; use Docker Compose instead.' }
# PostgreSQL's Windows tools cannot reliably initialize under a non-ASCII path.
$runtimePath = Join-Path $env:LOCALAPPDATA 'CatcareQA'
New-Item -ItemType Directory -Force -Path $runtimePath | Out-Null
$pgLink = Join-Path $runtimePath 'pgsql'
if (-not (Test-Path -LiteralPath $pgLink)) { New-Item -ItemType Junction -Path $pgLink -Target $pgSource | Out-Null }
$pgBin = Join-Path $pgLink 'bin'
Set-Location $runtimePath
$envValues = @{}
Get-Content -LiteralPath (Join-Path $taskRoot '.env') | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { $envValues[$matches[1]] = $matches[2] } }
$dataPath = Join-Path $runtimePath 'pgdata'
if (-not (Test-Path -LiteralPath (Join-Path $dataPath 'PG_VERSION'))) {
 $passwordFile = Join-Path $runtimePath 'init-password.tmp'
 try {
  [System.IO.File]::WriteAllText($passwordFile, $envValues.POSTGRES_PASSWORD, (New-Object System.Text.UTF8Encoding($false)))
  & (Join-Path $pgBin 'initdb.exe') -D $dataPath -U $envValues.POSTGRES_USER --pwfile=$passwordFile --auth=scram-sha-256 --encoding=UTF8 --locale=C
  if ($LASTEXITCODE -ne 0) { throw 'Database initialization failed.' }
 } finally { if (Test-Path -LiteralPath $passwordFile) { Remove-Item -LiteralPath $passwordFile } }
}
& (Join-Path $pgBin 'pg_ctl.exe') -D $dataPath status 2>$null
if ($LASTEXITCODE -ne 0) {
 & (Join-Path $pgBin 'pg_ctl.exe') -D $dataPath -l (Join-Path $runtimePath 'postgres.log') -o '-h 127.0.0.1 -p 55439' start
 if ($LASTEXITCODE -ne 0) { throw 'Database start failed.' }
}
$priorPassword = $env:PGPASSWORD
try {
 $env:PGPASSWORD=$envValues.POSTGRES_PASSWORD
 $exists = & (Join-Path $pgBin 'psql.exe') -h 127.0.0.1 -p 55439 -U $envValues.POSTGRES_USER -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'catcare'"
 if ($LASTEXITCODE -ne 0) { throw 'Database connection failed.' }
 if ($exists -ne '1') { & (Join-Path $pgBin 'createdb.exe') -h 127.0.0.1 -p 55439 -U $envValues.POSTGRES_USER catcare; if ($LASTEXITCODE -ne 0) { throw 'Database creation failed.' } }
} finally { $env:PGPASSWORD=$priorPassword }
Write-Output 'Local QA database ready at 127.0.0.1:55439'
