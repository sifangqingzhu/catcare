param([switch]$RemoveDemo)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$pgBin = Join-Path $env:LOCALAPPDATA 'CatcareQA\pgsql\bin'
if (-not (Test-Path -LiteralPath (Join-Path $pgBin 'psql.exe'))) { throw 'This helper targets the local QA PostgreSQL only.' }
$ids = @()
$idFile = Join-Path $taskRoot '.runtime\qa-users.json'
if (Test-Path -LiteralPath $idFile) { $ids = @(Get-Content -Raw -LiteralPath $idFile | ConvertFrom-Json) }
foreach ($id in $ids) { if ([string]$id -notmatch '^\d+$') { throw 'Invalid QA user ID.' } }
$idList = if ($ids.Count) { $ids -join ',' } else { '0' }
$predicate = "(id IN ($idList) AND username LIKE 'qa_%')"
if ($RemoveDemo) { $predicate += " OR username = 'demo'" }
# Resolve exact disposable owners, then remove only their resources in one transaction.
$sql = @"
BEGIN;
CREATE TEMP TABLE qa_owners AS SELECT id FROM users WHERE $predicate;
CREATE TEMP TABLE qa_tables AS SELECT id FROM tables WHERE owner_id IN (SELECT id FROM qa_owners);
DELETE FROM records WHERE table_id IN (SELECT id FROM qa_tables);
DELETE FROM weights WHERE table_id IN (SELECT id FROM qa_tables);
DELETE FROM boxes WHERE table_id IN (SELECT id FROM qa_tables);
DELETE FROM litters WHERE table_id IN (SELECT id FROM qa_tables);
DELETE FROM cats WHERE table_id IN (SELECT id FROM qa_tables);
DELETE FROM members WHERE table_id IN (SELECT id FROM qa_tables) OR user_id IN (SELECT id FROM qa_owners);
DELETE FROM tables WHERE id IN (SELECT id FROM qa_tables);
DELETE FROM sessions WHERE user_id IN (SELECT id FROM qa_owners);
DELETE FROM users WHERE id IN (SELECT id FROM qa_owners);
COMMIT;
"@
$envValues=@{}
Get-Content -LiteralPath (Join-Path $taskRoot '.env') | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { $envValues[$matches[1]]=$matches[2] } }
$priorPassword=$env:PGPASSWORD
try {
 $env:PGPASSWORD=$envValues.POSTGRES_PASSWORD
 $sql | & (Join-Path $pgBin 'psql.exe') -h 127.0.0.1 -p 55439 -U $envValues.POSTGRES_USER -d catcare -v ON_ERROR_STOP=1 -q
 if ($LASTEXITCODE -ne 0) { throw 'QA cleanup failed; no partial deletion was committed.' }
} finally { $env:PGPASSWORD=$priorPassword }
if (Test-Path -LiteralPath $idFile) { Remove-Item -LiteralPath $idFile }
Write-Output 'Disposable QA users/data cleaned up.'
