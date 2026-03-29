## Local Build And Test Workflow

This repository includes a local workflow where:

- source code remains on the host machine
- application build happens inside Docker
- the compiled binary is copied back into the repository under `.artifacts/`
- the Docker image and exported binary share the same build tag
- testing runs against a dedicated multi-container stack

This is the recommended workflow when you want reproducible local builds without installing the full Go and Node.js toolchain on the host.

## Requirements

- [Docker](https://docs.docker.com/get-docker/) with Docker Compose support
- a host filesystem checkout of the repository

The build and test scripts are:

- [`scripts/docker-build-local.sh`](../../../../scripts/docker-build-local.sh)
- [`scripts/local-test-stack.sh`](../../../../scripts/local-test-stack.sh)
- [`scripts/smoke-test.sh`](../../../../scripts/smoke-test.sh)

Make targets are also available in the project root `Makefile`.

## Build Artifacts

Local build artifacts are written to `.artifacts/`, which is ignored by git.

Relevant directories:

- `.artifacts/bin/` for exported application binaries
- `.artifacts/env/` for generated environment metadata
- `.artifacts/runtime/` for local runtime mounts used by the test stack

After a successful build, the generated metadata file `.artifacts/env/local.env` contains:

- `APP_TAG`
- `APP_IMAGE`
- `APP_BINARY`

The same tag is used for:

- the Docker image, for example `wgportal/wg-portal-local:<tag>`
- the exported binary, for example `.artifacts/bin/wg-portal-<tag>`

## Build The Application In Docker

Run:

```sh
make local-build
```

This does the following:

1. builds the application image with Docker
2. injects the build tag into the application version metadata
3. creates a stopped container from the built image
4. copies `/app/wg-portal` from the container into `.artifacts/bin/`
5. writes `.artifacts/env/local.env`

If you want to override the default image repository or tag:

```sh
IMAGE_REPO=myrepo/wg-portal-dev APP_TAG=my-feature-branch ./scripts/docker-build-local.sh
```

## Start The Local Test Stack

Run:

```sh
make local-test-up
```

This starts three containers:

- `db`: PostgreSQL used by the test environment
- `app`: the locally built WireGuard Portal image
- `toolbox`: a utility container for manual testing and automated checks

The test stack uses [`docker-compose.test.yml`](../../../../docker-compose.test.yml).

The application container mounts:

- `./docker/test/config.yml` as `/app/config/test.config.yml`
- `./.artifacts/runtime/data` as `/app/data`
- `./.artifacts/runtime/config` as `/etc/wireguard`

The test configuration disables startup behaviors that would require pre-existing WireGuard interfaces:

- `core.import_existing: false`
- `core.restore_state: false`

This keeps the local stack suitable for API and UI validation while still exercising the application inside its real runtime image.

## Run Smoke Tests

Run:

```sh
make local-test-smoke
```

This executes [`scripts/smoke-test.sh`](../../../../scripts/smoke-test.sh) inside the already running `toolbox` container.

The smoke test verifies:

- PostgreSQL connectivity from the toolbox container
- HTTP access to `GET /api`
- HTTP access to `GET /app/`

## Manual Testing

To inspect the environment manually, open a shell in the toolbox container:

```sh
./scripts/local-test-stack.sh shell
```

From there you can run commands such as:

```sh
curl -I http://app:8888/app/
curl -I http://app:8888/api
pg_isready -h db -p 5432 -U wgportal -d wgportal
psql "host=db port=5432 user=wgportal password=wgportal dbname=wgportal"
```

Published ports on the host:

- `8888/tcp` for the application UI and API
- `8787/tcp` for metrics
- `55432/tcp` for PostgreSQL

## Stop Or Reset The Stack

Stop containers and remove the network:

```sh
make local-test-down
```

Remove containers, network, and named volumes:

```sh
./scripts/local-test-stack.sh reset
```

## Notes

- The compose helper script sets an explicit project name because some local paths may contain spaces or non-ASCII characters.
- The workflow assumes Docker is available on the host and that the local Docker daemon can run privileged containers with `NET_ADMIN` and `NET_RAW`.
- This workflow is intended for local development and testing. It does not replace the production deployment guidance in [Docker](./docker.md) or [Binaries](./binaries.md).
