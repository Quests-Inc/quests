# Quests

A task manager in the spirit of a game quest log: web UI, API, CLI, check runner and a Wayland overlay (HUD + toasts) for niri / CachyOS.

Stack: **Go (API + CLI + runner) · PostgreSQL · Svelte 5 + TypeScript (Vite) · Python (MCP / Telegram / STT / overlay) · GTK4 + gtk4-layer-shell**

The web interface is in Russian, so interface labels are quoted as they appear on screen (with a gloss where needed).

How the parts fit together: [`docs/architecture.md`](docs/architecture.md). Terms: [`docs/glossary.md`](docs/glossary.md).

## Requirements

- Go ≥ 1.26 (builds the API, CLI and runner)
- Docker (local PostgreSQL: `just dev-db`)
- Python ≥ 3.12 and [uv](https://docs.astral.sh/uv/) (MCP server, Telegram bot, STT, overlay)
- [Bun](https://bun.sh) (frontend)
- [`just`](https://just.systems) (task runner)
- For the overlay: `gtk4`, `gtk4-layer-shell`, `python-gobject` (Wayland)

```bash
# Arch / CachyOS (overlay)
sudo pacman -S gtk4 gtk4-layer-shell python-gobject
```

## Setup

```bash
just bootstrap
```

`just` alone lists every recipe (build, run, `just check` = linters + tests).

`bootstrap` starts the local PostgreSQL in Docker, installs the Python dependencies (`uv sync --group dev --all-extras`) and the
frontend ones (`bun install`), creates `data/` and builds the Go CLI and API. The server applies the database schema itself when it starts.

## Running

| What | Command | URL / note |
|------|---------|------------|
| API + SPA | `just run-server` | http://127.0.0.1:8765 (the UI needs a built `frontend/dist`) |
| Build the API | `just build-server` | → `go/bin/quests-server` |
| Frontend (HMR) | `just run-frontend` | http://127.0.0.1:5173 (an API must be running) |
| Build the SPA | `just build-frontend` | → `frontend/dist` |
| Database schema | `just schema` | the server migrates by itself on start; `quests-server schema` prints the revision (needs `QUESTS_DATABASE_URL`; locally `just dev-db`) |
| CLI | `just quests …` / `go/bin/quests` | see [`docs/cli.md`](docs/cli.md) |
| Check runner | `just build-runner` → `go/bin/quests-runner` | executes check and emit commands on a device ([`docs/note-check.md`](docs/note-check.md)) |
| Overlay | `just run-overlay` | or `python -m overlay` |
| Telegram bot | `just run-telegram` | an HTTP client of the API |
| STT (voice → text) | `just run-stt` | sidecar of the bot, http://127.0.0.1:8766, needed for voice messages |
| API in Docker | `just dev-api` | container `quests-api-dev` on :8765 with its own PostgreSQL |
| Templates | the “Шаблоны” (Templates) page / `/api/templates` | daily/weekly → quest instances |

Overlay (IPC):

```bash
python -m overlay toggle    # passthrough ↔ interactive
python -m overlay monitor   # next monitor
python -m overlay status
```

The HUD theme and position are stored in `data/overlay.json` (`style_pack`, `monitor_connector`, `margins`). Switch the style in interactive mode (dropdown). The environment variable `QUESTS_STYLE_PACK` overrides the config at startup.

```bash
QUESTS_STYLE_PACK=cyberpunk just run-overlay
```

Layer-shell namespaces (for `niri msg layers` / `layer-rule`): `quests-overlay` (HUD), `quests-major`, `quests-minor`.

```kdl
// ~/.config/niri/config.kdl — example
layer-rule {
    match namespace="^quests-"
    // block-out-from "screencast"
}
```

## Access

A local instance on `127.0.0.1` works without a login. As soon as the database holds an account the whole API is locked
and the SPA asks for a login:

```bash
quests-server useradd <username>   # the password is asked for interactively
quests-server token add overlay    # a bearer token for the HUD / bot / CLI / MCP
```

An instance without accounts will not come up on a public address: the server refuses to bind anywhere but loopback.
Details: [`docs/auth.md`](docs/auth.md).

The database is PostgreSQL (`QUESTS_DATABASE_URL`); `just dev-db` starts one in Docker locally and the server applies the schema on start (see [`docs/postgres.md`](docs/postgres.md)).

## More

- User systemd units: [`deploy/systemd/`](deploy/systemd/)
- Deploying the server after a clone: [`deploy/SERVER.md`](deploy/SERVER.md)
- Docker images (api / bot / stt / frontend): [`deploy/docker/`](deploy/docker/)
- The HUD against a remote API (token, unit, niri): [`docs/hud-workstation.md`](docs/hud-workstation.md)
- Releases (SemVer) and production: [`deploy/README.md`](deploy/README.md). Production lives in the separate repository `Quests-Inc/quests-deploy` and is updated only by an explicit GitHub Release: `just release X.Y.Z`.
- Monitoring: [`docs/monitoring.md`](docs/monitoring.md); security model: [`docs/security.md`](docs/security.md)

> **Never run a bare `uv sync`.** It syncs the environment exactly and removes the optional packages (`mcp`, `aiogram`, `faster-whisper`),
> after which `quests-mcp`, the bot and STT fail with `No module named …`. Always `uv sync --group dev --all-extras` (which is what `just bootstrap` does).
