param([switch]$Demo)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$envPath = Join-Path $taskRoot '.env'
if (Test-Path -LiteralPath $envPath) { Write-Output '.env already exists; no credentials changed.'; exit 0 }
function New-Secret {
 $bytes = New-Object byte[] 32
 $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
 try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
 return ([BitConverter]::ToString($bytes)).Replace('-','').ToLowerInvariant()
}
$dbPassword = New-Secret
$sessionSecret = New-Secret
$demoPassword = New-Secret
$demoFlag = if ($Demo) { 'true' } else { 'false' }
$content = @"
POSTGRES_DB=catcare
POSTGRES_USER=catcare
POSTGRES_PASSWORD=$dbPassword
SESSION_SECRET=$sessionSecret
APP_ENV=development
WEB_BIND=127.0.0.1
WEB_PORT=8092
APP_ORIGIN=http://127.0.0.1:8092
ENABLE_DEMO=$demoFlag
DEMO_PASSWORD=$demoPassword
DATABASE_URL=postgres://catcare:${dbPassword}@127.0.0.1:55439/catcare?sslmode=disable
APP_HOST=127.0.0.1
API_PORT=8092
FRONTEND_DIR=../frontend/dist
"@
[System.IO.File]::WriteAllText($envPath, $content, (New-Object System.Text.UTF8Encoding($false)))
Write-Output '.env created with random credentials. Keep it private.'
