#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ -f .env ]; then printf '%s\n' '.env already exists; no credentials changed.'; exit 0; fi
umask 077
db_password=$(openssl rand -hex 32)
session_secret=$(openssl rand -hex 32)
cat > .env <<EOF
POSTGRES_DB=catcare
POSTGRES_USER=catcare
POSTGRES_PASSWORD=$db_password
SESSION_SECRET=$session_secret
APP_ENV=development
WEB_BIND=127.0.0.1
WEB_PORT=8092
APP_ORIGIN=http://127.0.0.1:8092
ENABLE_DEMO=false
DEMO_PASSWORD=
EOF
printf '%s\n' '.env created with random credentials. Keep it private.'
