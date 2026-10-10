# Quests task runner. `just` lists recipes; CI calls `just lint` / `just test`.

set shell := ["bash", "-euo", "pipefail", "-c"]

# The database every local recipe uses unless QUESTS_DATABASE_URL is set (`just dev-db` starts it).
export QUESTS_DATABASE_URL := env("QUESTS_DATABASE_URL", "postgresql://quests:quests@127.0.0.1:55433/quests?sslmode=disable")

default:
    @just --list

# --- setup -----------------------------------------------------------------

# Python deps (all extras), frontend deps, data dir, local PostgreSQL, Go binaries, migrations.
bootstrap: dev-db
    uv sync --group dev --all-extras
    cd frontend && npm install
    mkdir -p data
    just build-cli build-server

# Local PostgreSQL in Docker (127.0.0.1:55433, quests/quests); data survives restarts in a volume.
dev-db:
    docker compose -f deploy/docker/docker-compose.dev.yml up -d --wait postgres

# --- build -----------------------------------------------------------------

# API → go/bin/quests-server (QUESTS_SERVER_OUT overrides the path)
build-server:
    cd go && go build -o "${QUESTS_SERVER_OUT:-bin/quests-server}" ./cmd/quests-server

# CLI → go/bin/quests
build-cli:
    cd go && go build -o "${QUESTS_CLI_OUT:-bin/quests}" ./cmd/quests

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

# The API on :8765 (local PostgreSQL via `just dev-db`; the server migrates it on start); needs a built SPA for the UI.
run-server: dev-db build-server
    QUESTS_ROOT="${QUESTS_ROOT:-{{ justfile_directory() }}}" \
    QUESTS_DATA_DIR="${QUESTS_DATA_DIR:-{{ justfile_directory() }}/data}" \
    QUESTS_HOST="${QUESTS_HOST:-127.0.0.1}" \
    QUESTS_PORT="${QUESTS_PORT:-8765}" \
    ./go/bin/quests-server

# Vite dev server with HMR on :5173 (proxies /api to a running API)
run-frontend:
    cd frontend && npm run dev

run-telegram:
    uv run quests-telegram

# Speech-to-text sidecar for the bot on :8766
run-stt:
    uv run quests-stt

# Wayland HUD (needs gtk4, gtk4-layer-shell, python-gobject; installs pywayland into overlay/_vendor once)
run-overlay:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! pacman -Q gtk4-layer-shell &>/dev/null; then
        echo "Missing gtk4-layer-shell. Install with: sudo pacman -S gtk4-layer-shell" >&2
        exit 1
    fi
    vendor="{{ justfile_directory() }}/overlay/_vendor"
    if [[ ! -d "$vendor/pywayland" ]]; then
        mkdir -p "$vendor"
        uv pip install --target "$vendor" pywayland >/dev/null 2>&1 || true
    fi
    export PYTHONPATH="${vendor}${PYTHONPATH:+:$PYTHONPATH}"
    cd "{{ justfile_directory() }}"
    exec python3 -m overlay

# The CLI: `just quests list --json` (builds it first)
quests *args: build-cli
    ./go/bin/quests {{ args }}

# Local API in Docker (container quests-api-dev on :8765 with its own PostgreSQL); --rebuild after Go/Dockerfile changes.
dev-api *args:
    #!/usr/bin/env bash
    set -euo pipefail
    rebuild=0
    for arg in {{ args }}; do
        case "$arg" in
            --rebuild|-b) rebuild=1 ;;
            *) echo "usage: just dev-api [--rebuild]" >&2; exit 2 ;;
        esac
    done
    compose=(docker compose -f deploy/docker/docker-compose.dev.yml)
    # Free :8765 from a host binary / old container.
    if command -v lsof >/dev/null 2>&1; then
        pids="$(lsof -t -iTCP:8765 -sTCP:LISTEN 2>/dev/null || true)"
        if [[ -n "$pids" ]]; then
            echo "stopping host listeners on :8765: $pids" >&2
            # shellcheck disable=SC2086
            kill $pids 2>/dev/null || true
            sleep 0.4
        fi
    fi
    docker rm -f quests-api-dev >/dev/null 2>&1 || true
    if [[ ! -d frontend/dist ]]; then
        echo "frontend/dist missing — building SPA…" >&2
        (cd frontend && npm run build)
    fi
    if [[ "$rebuild" -eq 1 ]]; then
        "${compose[@]}" up -d --build --force-recreate api
    else
        # Use the existing image if present; build once otherwise.
        docker image inspect quests-api:local >/dev/null 2>&1 || "${compose[@]}" build api
        "${compose[@]}" up -d --force-recreate api
    fi
    echo -n "waiting for /api/ping" >&2
    for _ in $(seq 1 40); do
        if curl -fsS --noproxy '*' http://127.0.0.1:8765/api/ping >/dev/null 2>&1; then
            echo " ok" >&2
            curl -fsS --noproxy '*' http://127.0.0.1:8765/api/health | head -c 120
            echo
            exit 0
        fi
        echo -n . >&2
        sleep 0.5
    done
    echo " timeout" >&2
    "${compose[@]}" logs --tail 80 api >&2 || true
    exit 1

# Pull the prod database (+ icons) into the local dev PostgreSQL and ./data. Flags: --start, --pull-image.
# Needs QUESTS_PROD_SSH=user@host, docker on both sides, and `just dev-db` running. REPLACES the dev database's contents.
pull-prod-data *args:
    #!/usr/bin/env bash
    set -euo pipefail

    ROOT="{{ justfile_directory() }}"
    DATA="${QUESTS_DATA_DIR:-$ROOT/data}"
    SSH_HOST="${QUESTS_PROD_SSH:?set QUESTS_PROD_SSH, e.g. user@host}"
    API_CONTAINER="${QUESTS_PROD_CONTAINER:-quests-api}"
    PG_CONTAINER="${QUESTS_PROD_PG_CONTAINER:-quests-postgres}"
    LOCAL_PG="${QUESTS_LOCAL_PG_CONTAINER:-quests-pg-dev}"
    IMAGE="${QUESTS_API_IMAGE:-ghcr.io/valirum/quests-api:main}"

    START=0
    PULL_IMAGE=0
    for arg in {{ args }}; do
      case "$arg" in
        --start) START=1 ;;
        --pull-image) PULL_IMAGE=1 ;;
        -h|--help)
          cat <<DOC
    usage: just pull-prod-data [--pull-image] [--start]

      Dump the database of \`$PG_CONTAINER\` on \`$SSH_HOST\` into the local container \`$LOCAL_PG\`
      (replacing its data) and copy /app/data of \`$API_CONTAINER\` into \`$DATA\`.

    env:
      QUESTS_PROD_SSH           required, e.g. user@host
      QUESTS_PROD_CONTAINER     api container on prod, default: quests-api
      QUESTS_PROD_PG_CONTAINER  postgres container on prod, default: quests-postgres
      QUESTS_LOCAL_PG_CONTAINER local dev postgres, default: quests-pg-dev
      QUESTS_DATA_DIR           default: <repo>/data
      QUESTS_API_IMAGE          default: ghcr.io/valirum/quests-api:main
    DOC
          exit 0
          ;;
        *)
          echo "unknown arg: $arg (try --help)" >&2
          exit 2
          ;;
      esac
    done

    docker inspect "$LOCAL_PG" >/dev/null 2>&1 || { echo "local database container $LOCAL_PG is not running: just dev-db" >&2; exit 1; }
    mkdir -p "$DATA"

    echo "dumping $SSH_HOST:$PG_CONTAINER → $LOCAL_PG" >&2
    docker exec "$LOCAL_PG" psql -U quests -d postgres -qc "DROP DATABASE IF EXISTS quests WITH (FORCE)" -c "CREATE DATABASE quests"
    ssh -o BatchMode=yes "$SSH_HOST" "docker exec $PG_CONTAINER pg_dump -U quests --format=custom --no-owner quests" \
      | docker exec -i "$LOCAL_PG" pg_restore -U quests -d quests --no-owner

    notes="$(docker exec "$LOCAL_PG" psql -U quests -d quests -Atc 'SELECT COUNT(*) FROM note' 2>/dev/null || echo '?')"
    quests="$(docker exec "$LOCAL_PG" psql -U quests -d quests -Atc 'SELECT COUNT(*) FROM quest' 2>/dev/null || echo '?')"
    echo "ok: database restored (notes=$notes quests=$quests)" >&2

    # Keep workstation overlay HUD config if present.
    overlay_bak=""
    if [[ -f "$DATA/overlay.json" ]]; then
      overlay_bak="$(mktemp)"
      cp -a "$DATA/overlay.json" "$overlay_bak"
    fi
    echo "pulling $SSH_HOST:$API_CONTAINER:/app/data → $DATA" >&2
    ssh -o BatchMode=yes "$SSH_HOST" "docker exec $API_CONTAINER tar czf - -C /app/data ." | tar xzf - -C "$DATA"
    if [[ -n "$overlay_bak" ]]; then
      mv -f "$overlay_bak" "$DATA/overlay.json"
    fi

    if [[ "$PULL_IMAGE" -eq 1 ]]; then
      echo "pulling $IMAGE → quests-api:local" >&2
      if docker pull "$IMAGE"; then
        docker tag "$IMAGE" quests-api:local
      else
        echo "ghcr pull failed — docker save|load from prod" >&2
        ssh -o BatchMode=yes "$SSH_HOST" "docker save $IMAGE" | docker load
        docker tag "$IMAGE" quests-api:local
      fi
    fi

    if [[ "$START" -eq 1 ]]; then
      exec just dev-api
    fi

    echo "next: just dev-api   # UI http://127.0.0.1:8765/" >&2


# Schema revision of QUESTS_DATABASE_URL vs. what this build expects (the server migrates itself on start).
schema: build-server
    ./go/bin/quests-server schema

# --- release & deploy -------------------------------------------------------

# Publish release VERSION (e.g. `just release 1.4.0`) from the tip of main: the tag, the images, and — after the
# `production` environment's approval — the deployment (deploy/README.md). Needs `gh` and main's CI to be green.
release version:
    #!/usr/bin/env bash
    set -euo pipefail
    v="{{ version }}"
    [[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "not a SemVer version: $v" >&2; exit 2; }
    tip="$(git rev-parse origin/main)"
    conclusion="$(gh run list --workflow ci.yml --branch main --commit "$tip" --json conclusion -q '.[0].conclusion // "none"')"
    [ "$conclusion" = success ] || { echo "CI on main ($tip) is '$conclusion', not success" >&2; exit 1; }
    pre=(); [[ "$v" == *-* ]] && pre=(--prerelease)
    gh release create "v$v" --target main --title "v$v" --generate-notes "${pre[@]}"

# Render the production compose with stand-in values: catches a broken stack before a release does.
check-deploy:
    #!/usr/bin/env bash
    set -euo pipefail
    tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
    cp -a deploy/prod/. "$tmp/"
    echo "QUESTS_VERSION=0.0.0" > "$tmp/version.env"
    cd "$tmp" && docker compose --env-file version.env --env-file config.env \
        -f docker-compose.yml -f docker-compose.monitoring.yml --profile bot config -q
    echo "production compose renders"

# One-time: create the age key (kept outside the repo), put its public half into deploy/prod/.sops.yaml and
# encrypt a first secrets.enc.env from the template. The private key is what bootstrap needs as SOPS_AGE_KEY.
secrets-init:
    #!/usr/bin/env bash
    set -euo pipefail
    key="${SOPS_AGE_KEY_FILE:-$HOME/.config/sops/age/keys.txt}"
    if [ ! -f "$key" ]; then mkdir -p "$(dirname "$key")"; age-keygen -o "$key"; fi
    pub="$(age-keygen -y "$key")"
    cd deploy/prod
    sed -i.bak "s|age: .*|age: $pub|" .sops.yaml && rm -f .sops.yaml.bak
    if [ ! -f secrets.enc.env ]; then
        SOPS_AGE_KEY_FILE="$key" sops encrypt --filename-override secrets.enc.env --input-type dotenv --output-type dotenv secrets.example.env > secrets.enc.env.tmp
        mv secrets.enc.env.tmp secrets.enc.env
        echo "created deploy/prod/secrets.enc.env: fill it in with 'just secrets-edit'"
    fi
    echo "public key: $pub"
    echo "private key file: $key  (keep it safe; its contents are the SOPS_AGE_KEY secret of the bootstrap)"

# Edit the production secrets (decrypts into $EDITOR, encrypts on save).
secrets-edit:
    cd deploy/prod && SOPS_AGE_KEY_FILE="${SOPS_AGE_KEY_FILE:-$HOME/.config/sops/age/keys.txt}" sops --input-type dotenv --output-type dotenv secrets.enc.env

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
    shellcheck --severity=warning deploy/docker/*.sh deploy/doco-cd/*.sh deploy/prod/*.sh deploy/prod/monitoring/*.sh

# Auto-fix what ruff/gofmt can fix.
fix:
    uv run ruff check . --fix
    cd go && gofmt -w .

test: test-go test-py test-frontend

# Go tests run against a real PostgreSQL (QUESTS_TEST_PG_URL). Without it set, a throwaway
# one is started in Docker (container quests-pg-test, left running between runs).
test-go:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "${QUESTS_TEST_PG_URL:-}" ]; then
        just pg-test >/dev/null
        export QUESTS_TEST_PG_URL="postgresql://quests:test@127.0.0.1:55432/quests?sslmode=disable"
    fi
    cd go && go build ./... && go test ./...

# Throwaway PostgreSQL for the Go tests (idempotent); stop with `docker stop quests-pg-test`.
pg-test:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "$(docker ps -q -f name=^quests-pg-test$)" ]; then
        docker run -d --rm --name quests-pg-test -e POSTGRES_USER=quests -e POSTGRES_PASSWORD=test \
            -e POSTGRES_DB=quests -p 127.0.0.1:55432:5432 postgres:17 >/dev/null
    fi
    until docker exec quests-pg-test pg_isready -U quests >/dev/null 2>&1; do sleep 0.5; done
    echo 'export QUESTS_TEST_PG_URL="postgresql://quests:test@127.0.0.1:55432/quests?sslmode=disable"'

test-py:
    uv run --all-extras --group dev pytest

test-frontend:
    cd frontend && npm ci && npm test && npm run build
