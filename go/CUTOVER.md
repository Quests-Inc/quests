# Go API cutover — done

Default API is the Go binary (`go/bin/quests-server` via `./scripts/run-server.sh` on `:8765`).

## Run

```bash
just build-server build-cli
./scripts/run-server.sh          # :8765
./scripts/quests list
```

SPA: build `frontend/dist` (or use Vite proxy to the API).

## Still Python

- Alembic migrations (`quests-migrate`)
- Telegram bot, MCP, overlay, STT
- CLI `llm-add` (Go shells into Python)

## Docker

`deploy/docker/Dockerfile` api stage: Go binary + SPA + Python only for migrate on entrypoint.
