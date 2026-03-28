#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="${ROOT_DIR}/.artifacts/env/local.env"
COMPOSE_FILE="${ROOT_DIR}/docker-compose.test.yml"
PROJECT_NAME="${COMPOSE_PROJECT_NAME:-wg-portal-test}"

ensure_build() {
  if [ ! -f "${ENV_FILE}" ]; then
    "${ROOT_DIR}/scripts/docker-build-local.sh"
  fi
}

compose() {
  docker compose \
    --project-name "${PROJECT_NAME}" \
    --env-file "${ENV_FILE}" \
    -f "${COMPOSE_FILE}" \
    "$@"
}

COMMAND="${1:-up}"

case "${COMMAND}" in
  up)
    ensure_build
    compose up -d --build
    ;;
  down)
    ensure_build
    compose down --remove-orphans
    ;;
  reset)
    ensure_build
    compose down --volumes --remove-orphans
    ;;
  ps)
    ensure_build
    compose ps
    ;;
  logs)
    ensure_build
    shift || true
    compose logs -f "$@"
    ;;
  smoke)
    ensure_build
    compose exec -T toolbox ./scripts/smoke-test.sh
    ;;
  shell)
    ensure_build
    compose exec toolbox bash
    ;;
  *)
    echo "Usage: $0 {up|down|reset|ps|logs|smoke|shell}" >&2
    exit 1
    ;;
esac
