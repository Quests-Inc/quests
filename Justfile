# Quests task runner. `just` lists recipes; CI calls `just lint` / `just test`.

set shell := ["bash", "-euo", "pipefail", "-c"]

default:
    @just --list

# --- setup -----------------------------------------------------------------

# Python deps (all extras), frontend deps, data dir, migrations, Go binaries.
bootstrap:
    uv sync --group dev --all-extras
    cd frontend && npm install
    mkdir -p data
    just build-cli build-server
    ./go/bin/quests-server migrate

# --- build -----------------------------------------------------------------

build-server:
    ./scripts/build-server.sh

build-cli:
    ./scripts/build-cli.sh

# Check runner (executes checks on the device) → go/bin/quests-runner
build-runner:
    #!/usr/bin/env bash
    set -euo pipefail
    version="${QUESTS_RUNNER_VERSION:-$(git describe --always --dirty 2>/dev/null || echo dev)}"
    cd go
    CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$version" -o "${QUESTS_RUNNER_OUT:-bin/quests-runner}" ./cmd/quests-runner

# SPA → frontend/dist (served by the API)
build-frontend:
    cd frontend && npm run build

# --- run -------------------------------------------------------------------

run-server:
    ./scripts/run-server.sh

run-frontend:
    ./scripts/run-frontend.sh

run-telegram:
    ./scripts/run-telegram.sh

run-stt:
    ./scripts/run-stt.sh

run-overlay:
    ./scripts/run-overlay-smoke.sh

# Local API in Docker; pass --rebuild after Go/Python/Dockerfile changes.
dev-api *args:
    ./scripts/dev-api-docker.sh {{ args }}

# Apply schema migrations (uses QUESTS_DATABASE_URL, else data/quests.db); `just migrate current` prints the revision.
migrate *args: build-server
    ./go/bin/quests-server migrate {{ args }}

# --- quality ---------------------------------------------------------------

# Everything CI checks, in the same order.
check: lint test

lint: lint-go lint-py lint-sh

lint-go:
    cd go && test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }
    cd go && go vet ./...
    cd go && golangci-lint run ./...

lint-py:
    uv run ruff check .

lint-sh:
    shellcheck --severity=warning scripts/*.sh scripts/quests deploy/docker/*.sh deploy/monitoring/*.sh

# Auto-fix what ruff/gofmt can fix.
fix:
    uv run ruff check . --fix
    cd go && gofmt -w .

test: test-go test-py test-frontend

# DB tests need PostgreSQL: set QUESTS_TEST_PG_URL (e.g. `just pg-test` starts one in Docker).
test-go:
    cd go && go build ./... && go test ./...

# Throwaway PostgreSQL for the DB tests; prints the URL to export.
pg-test:
    docker run -d --rm --name quests-pg-test -e POSTGRES_USER=quests -e POSTGRES_PASSWORD=test -e POSTGRES_DB=quests -p 127.0.0.1:55432:5432 postgres:17
    @echo 'export QUESTS_TEST_PG_URL="postgresql://quests:test@127.0.0.1:55432/quests?sslmode=disable"'
    @echo 'stop with: docker stop quests-pg-test'

test-py:
    uv run --all-extras --group dev pytest

test-frontend:
    cd frontend && npm ci && npm test && npm run build
