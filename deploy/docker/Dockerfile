# syntax=docker/dockerfile:1

# --- Frontend (Vite / Svelte) ---
FROM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- Go API (+ CLI) ---
FROM golang:1.26-bookworm AS go-build
# The image's Go is newer than go.mod's patch, so never fetch another toolchain
# at build time (fail loudly instead if that ever stops being true).
ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0
WORKDIR /src
COPY go/go.mod go/go.sum ./
RUN go mod download
# Compile the third-party deps into the build
# cache in a layer keyed only by go.mod/go.sum, so a source-only change below
# recompiles just our own packages. Flags must match the real build (-trimpath).
# The import list only affects speed: if it goes stale the prime step is skipped.
RUN <<'PRIME'
mkdir _prime
cat > _prime/main.go <<'GO'
package main

import (
	_ "github.com/gabriel-vasile/mimetype"
	_ "github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/gorilla/websocket"
	_ "github.com/klauspost/compress/zstd"
	_ "github.com/pkg/sftp"
	_ "golang.org/x/crypto/ssh"
	_ "golang.org/x/term"
)

func main() {}
GO
go build -trimpath -o /dev/null ./_prime || echo "deps prime skipped"
PRIME
COPY go/ ./
RUN go build -trimpath -ldflags="-s -w" -o /out/quests-server ./cmd/quests-server \
 && go build -trimpath -ldflags="-s -w" -o /out/quests ./cmd/quests

# --- Python (migrations + bot tooling) ---
FROM python:3.12-slim-bookworm AS python-base
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1 \
    PATH="/app/.venv/bin:$PATH" \
    QUESTS_ROOT=/app

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:0.12.19 /uv /usr/local/bin/uv

WORKDIR /app
COPY pyproject.toml uv.lock README.md ./
COPY src ./src

# Core only (dotenv/tzdata) for the bot and STT images below.
RUN uv sync --locked --no-dev

# --- API runtime: Go server + SPA. Migrations are embedded (`quests-server migrate`), PDF export is
# in-process Go (go/internal/pdfdoc): no project Python here. python3 itself stays only because users
# write server-side check/emit commands with a `#!/usr/bin/env python3` shebang (docs/note-check.md). ---
FROM python:3.12-slim-bookworm AS api
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
# pg_dump for PostgreSQL backups: the client must be at least the server's major
# version, which Debian's own package is not — take it from the PGDG repo.
RUN apt-get update \
    && apt-get install -y --no-install-recommends postgresql-common \
    && /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y \
    && apt-get install -y --no-install-recommends postgresql-client-17 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-build /out/quests-server /usr/local/bin/quests-server
COPY --from=go-build /out/quests /usr/local/bin/quests
COPY --from=frontend /src/frontend/dist /app/frontend/dist
COPY deploy/docker/entrypoint-api.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
# `quests` on PATH is the Go CLI, so check_command steps can run e.g. `quests progress N`.
ENV QUESTS_HOST=0.0.0.0 \
    QUESTS_PORT=8765 \
    QUESTS_ROOT=/app \
    QUESTS_DATA_DIR=/app/data
EXPOSE 8765
VOLUME ["/app/data"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["quests-server"]

# --- Telegram bot: aiogram only — STT lives in its own image/container ---
FROM python-base AS bot
RUN uv sync --locked --no-dev --extra bot
COPY deploy/docker/entrypoint-bot.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/app/data"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["quests-telegram"]

# --- STT sidecar: faster-whisper (ctranslate2/onnxruntime/av) isolated here
# so it only rebuilds/redownloads when STT itself changes, not on every
# aiogram bump. Bot talks to it over HTTP (quests.telegram.sttclient). ---
FROM python-base AS stt
RUN uv sync --locked --no-dev --extra stt
COPY deploy/docker/entrypoint-stt.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENV QUESTS_STT_HOST=0.0.0.0 \
    QUESTS_STT_PORT=8766 \
    HF_HOME=/app/data/hf-cache
EXPOSE 8766
# Whisper model weights download here on first use (faster-whisper → HF
# cache, redirected into the volume via HF_HOME) — a redeploy/restart then
# doesn't refetch them.
VOLUME ["/app/data"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["quests-stt"]
