# Go API cutover — done

Default API is the Go binary (`go/bin/quests-server` via `just run-server` on `:8765`).

## Run

```bash
just build-server build-cli
just run-server          # :8765
just quests list
```

SPA: build `frontend/dist` (or use Vite proxy to the API).

## Still Python

- Telegram bot, MCP, overlay, STT
- CLI `llm-add` (Go shells into Python)

## Docker

`deploy/docker/Dockerfile` api stage: Go binary + SPA + Python only for migrate on entrypoint.
