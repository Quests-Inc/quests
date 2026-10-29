# syntax=docker/dockerfile:1

# --- Frontend (Vite / Svelte) ---
FROM oven/bun:1.4.2-debian AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

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
# VERSION is the release (1.2.3), stamped into `quests-server version` and /api/health.
ARG VERSION=dev
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/quests-server ./cmd/quests-server \
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

# --- API runtime: Go server + SPA. The server migrates its own schema (embedded migrations), PDF export is
# in-process Go (go/internal/pdfdoc), and the server executes no user shell: check/emit/hook Code run on
# runners (quests-runner). Trusted sinks may dial HTTP/unix; pg_dump is for backups. ---
FROM debian:bookworm-slim AS api
LABEL org.opencontainers.image.source="https://github.com/Quests-Inc/quests"
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates tzdata poppler-utils \
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
# `quests` (the Go CLI) is on PATH for the operator's hook scripts.
WORKDIR /app
ENV QUESTS_HOST=0.0.0.0 \
    QUESTS_PORT=8765 \
    QUESTS_ROOT=/app \
    QUESTS_DATA_DIR=/app/data
EXPOSE 8765
RUN mkdir -p /app/data
VOLUME ["/app/data"]
# The server migrates the schema itself before it starts serving.
CMD ["quests-server"]

# --- Telegram bot: aiogram only — STT lives in its own image/container ---
FROM python-base AS bot
LABEL org.opencontainers.image.source="https://github.com/Quests-Inc/quests"
RUN uv sync --locked --no-dev --extra bot
COPY deploy/docker/entrypoint-bot.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/app/data"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["quests-telegram"]
