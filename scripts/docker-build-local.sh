#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ARTIFACTS_DIR="${ROOT_DIR}/.artifacts"
BIN_DIR="${ARTIFACTS_DIR}/bin"
ENV_DIR="${ARTIFACTS_DIR}/env"
RUNTIME_DIR="${ARTIFACTS_DIR}/runtime"

mkdir -p "${BIN_DIR}" "${ENV_DIR}" "${RUNTIME_DIR}/data" "${RUNTIME_DIR}/config"

default_tag() {
  if git -C "${ROOT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || git -C "${ROOT_DIR}" rev-parse --short HEAD
  else
    date +%Y%m%d%H%M%S
  fi
}

sanitize_tag() {
  printf '%s' "$1" | tr '/ :' '---'
}

APP_TAG="${APP_TAG:-$(default_tag)}"
APP_TAG=$(sanitize_tag "${APP_TAG}")
IMAGE_REPO="${IMAGE_REPO:-wgportal/wg-portal-local}"
APP_IMAGE="${IMAGE_REPO}:${APP_TAG}"
APP_BINARY="${BIN_DIR}/wg-portal-${APP_TAG}"
ENV_FILE="${ENV_DIR}/local.env"

echo "Building image ${APP_IMAGE}"
docker build \
  --build-arg BUILD_VERSION="${APP_TAG}" \
  -t "${APP_IMAGE}" \
  "${ROOT_DIR}"

CONTAINER_ID=$(docker create "${APP_IMAGE}")
cleanup() {
  docker rm -f "${CONTAINER_ID}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker cp "${CONTAINER_ID}:/app/wg-portal" "${APP_BINARY}"
chmod +x "${APP_BINARY}"

cat > "${ENV_FILE}" <<EOF
APP_TAG=${APP_TAG}
APP_IMAGE=${APP_IMAGE}
APP_BINARY=${APP_BINARY}
EOF

echo "Binary exported to ${APP_BINARY}"
echo "Build metadata written to ${ENV_FILE}"
