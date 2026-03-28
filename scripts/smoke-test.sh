#!/usr/bin/env sh
set -eu

APP_BASE_URL="${APP_BASE_URL:-http://app:8888}"
POSTGRES_HOST="${POSTGRES_HOST:-db}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_DB="${POSTGRES_DB:-wgportal}"
POSTGRES_USER="${POSTGRES_USER:-wgportal}"

echo "Checking PostgreSQL connectivity"
pg_isready -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}"

echo "Checking application endpoints"
curl --fail --silent --show-error --max-time 10 "${APP_BASE_URL}/api" >/dev/null
curl --fail --silent --show-error --max-time 10 "${APP_BASE_URL}/app/" >/dev/null

echo "Smoke test passed"
