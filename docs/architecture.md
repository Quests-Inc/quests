# Architecture

What Quests consists of, what each part is written in and how the parts talk to each other. The structure of the Go core and of the frontend, and the dependency rules, are in [ADR-0001](adr/0001-architecture.md); terms are in the [glossary](glossary.md).

## Components

| Component | Language | Where | What it does |
|---|---|---|---|
| **API server** `quests-server` | Go | `go/cmd/quests-server`, `go/internal/*` | HTTP API, serves the SPA, scheduler (templates, note checks, backups), events; applies the database schema itself on start |
| **CLI** `quests` | Go | `go/cmd/quests` | Terminal client of the API (`docs/cli.md`) |
| **Runner** `quests-runner` | Go | `go/cmd/quests-runner` | Executes check and emit commands on the owner's device; the server runs no commands (`docs/security.md`, rules in `docs/automation.md`) |
| **Web UI** | Svelte 5 + TypeScript | `frontend/` | SPA; the API response types are generated from Go (`just gen-types`) |
| **MCP server** `quests-mcp` | Go | `go/cmd/quests-mcp`, `go/internal/mcp` | Tools for AI agents on top of the HTTP API (stdio; compact paged lists) |
| **Telegram bot** | Python | `src/quests/telegram/` | Notifications, quick add, the action assistant |
| **Overlay** | Go + gotk4 + gtk4-layer-shell | `go/cmd/quests-overlay`, `go/internal/overlay/` | Wayland HUD and toasts on a workstation (`just run-overlay`) |
| **Android HUD** | Kotlin | `android/` | A foreground service with an ongoing notification on top of the same REST API |

Everything that is not the API talks to the server **only over HTTP** (`/api/*`, token or session): none of them has direct access to the database.

## The Go core

| Package | What is inside |
|---|---|
| `identity`, `identity/http` | accounts, sessions, MFA, tokens, workspaces, roles |
| `knowledge`, `knowledge/http` | notes, attachments, revisions, search, quotas |
| `journal`, `journal/http` | quests, steps, chains, categories, tags, the change log |
| `automation`, `automation/http` | templates, runners and their jobs, secrets, emit attempts |
| `schedule` | the scheduler: decides *when* (template materialization, expiry, checks, backups, purging); reaches the database only through the contexts' stores |
| `httpapi` | the composition root: wires the contexts' handlers and the cross-cutting slices, authentication, PDF rendering |
| `assistant` | cross-cutting slice: an account's AI assistant settings and action batches built from text |
| `graphdraft` | cross-cutting slice: the draft of the relations map (operations accumulate on the server, committing applies them through the API) |
| `wsarchive` | cross-cutting slice: the workspace archive (zip export and import) |
| `store` | the remainder: cross-context reads (graph, export/import, statistics) and the backup log |
| `kernel`, `refs`, `platform/*` | the shared kernel, `kind=ID` references, technical infrastructure (database, events, rate limiting, apiclient…) |
| `cli`, `llmassist` | API clients: the command line and the AI assistant (the shared HTTP client is `platform/apiclient`) |

The frontend is organized in Feature-Sliced Design layers (`frontend/README.md`).

## Data

PostgreSQL 17 is the only database (`docs/postgres.md`). Workspaces are isolated by row-level security under the role `quests_app`
(`docs/security.md`). The schema is embedded SQL migrations in `go/internal/platform/db/sql/pg`; the applied revision is kept in the table `alembic_version`
(the name is a leftover from Alembic, which once created the production database; Alembic itself is not in the repository).

## Images and rollout

Two images are built from one `deploy/docker/Dockerfile`: `quests-api` (the Go binary + SPA, no interpreters),
`quests-bot` (Python). A release `vX.Y.Z` → images in GHCR → a pull request in the `quests-deploy` repository → doco-cd on the
server picks the change up (`deploy/README.md`).

## What stays in Python and why

The bot is a separate process with its own life cycle and dependencies (aiogram).
There are no plans to rewrite them for the sake of a single language: the boundary is the HTTP API, and they affect neither the data schema nor
the API image. There is no Python in the API image at all. The Wayland overlay (`quests-overlay`) and the MCP server (`quests-mcp`) are Go.

## Gates against regressions

All gates run locally (`just test`, `cd frontend && bun run verify`) and, with the same commands, in CI.

| Gate | What it holds |
|---|---|
| `golangci-lint` + depguard (`go/.golangci.yml`) | the direction of Go package dependencies (contexts, `platform`, the root) |
| `internal/archtest` | a non-test Go file is at most 600 lines |
| `tools/coverage_gate.py` (+ `go/.coverage-baseline.json`) | Go coverage per package does not drop; a new package gets at least 50 %; integration tests count for the handlers (`-coverpkg`) |
| `go tool tygo generate` with no diff | the frontend types match the Go structs |
| `check:fsd` | FSD layers, imports only through `index.ts`, cross-imports between entities only through `@x` |
| `check:arch` (+ `frontend/.arch-baseline.json`) | a file does not grow past its bar, a new one is ≤ 800 lines; `model/` and `lib/` know nothing about components, `.svelte.ts` controllers live only there; the network only in `shared/api`; **every file of `model/` and `shared/lib/` is covered by a test** |
| `check` (svelte-check) and `check:any` | the number of type errors and explicit `any` does not grow |
| `tests/unit`, `tests/components`, `tests/security` | pure logic; components, controllers and a walk through the whole app over a fake backend (catches looping effects and broken tabs); the markdown sanitizer on jsdom |

New bars are set with the `--update` commands (`check:any`, `check:arch`, `coverage_gate.py`) and only downwards: debt may be paid off, never increased.
