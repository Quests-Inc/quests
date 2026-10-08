# PostgreSQL

The API runs on PostgreSQL when `QUESTS_DATABASE_URL` (`postgresql://user:pw@host:port/db`) is set,
and on the SQLite file in `QUESTS_DATA_DIR` otherwise. Both `quests-migrate` and `quests-server` read it.

## Schema chains

| DB | Migrations | Notes |
|---|---|---|
| SQLite | `alembic/versions/` | legacy chain, frozen after cutover |
| PostgreSQL | `alembic/versions_pg/` | `1a2b3c4d5e6f` is a baseline equal to the SQLite head, `c7e1a9b3d5f2` adds workspaces; new revisions go here, PG-only |

After a new PG revision, append its id to `alembicOrder`/`expectedAlembic` in `go/internal/db/db.go`.

Baseline conventions (kept so the Go store works unchanged on both):
boolean-ish columns are `INTEGER` 0/1, timestamps are naive-UTC `TIMESTAMP`, `VARCHAR(n)` is `TEXT`
(prod data already exceeds the declared lengths).

## Local

```bash
export QUESTS_DATABASE_URL=postgresql://quests@127.0.0.1:5432/quests
uv run quests-migrate
./scripts/run-server.sh
```

Go tests that need a real PG (`go/internal/db`, applies the baseline into a throwaway schema):

```bash
QUESTS_TEST_PG_URL=postgresql://quests@127.0.0.1:5432/quests go test ./go/internal/db
```

## Cutover from the SQLite prod

1. Add `POSTGRES_PASSWORD` to `.env`; start only the DB:
   `docker compose --env-file .env -f deploy/docker/docker-compose.yml -f deploy/docker/docker-compose.pg.yml up -d postgres`
2. Stop the API (`stop api bot`) so SQLite is quiescent.
3. Migrate PG (to head, workspaces included) and copy the data into workspace 1 (from a host with the volume and uv; `QUESTS_DATABASE_URL` points at the new DB):
   ```bash
   uv run quests-migrate
   uv run python scripts/sqlite_to_pg.py path/to/quests.db "$QUESTS_DATABASE_URL"
   ```
   The script refuses a non-empty target, compares row counts per table and rolls back on any mismatch.
4. Start the stack with the overlay (`up -d`). The API logs the backend; `/api/health` shows the backup probe.
5. Keep `quests.db` and its latest `backups/*.db` as the rollback: drop the overlay and start as before
   (anything written after the cutover is not in SQLite).

## Backups

With PG the periodic job runs `pg_dump --format=custom` (file `quests-<ts>.dump`), then archives attachments,
uploads and rotates exactly as before. The api image ships `pg_dump` 17 from PGDG. Restore:

```bash
createdb -h HOST -U quests quests_restored
pg_restore -h HOST -U quests --no-owner -d quests_restored quests-<ts>.dump
```

## Workspaces (quest=213)

PostgreSQL only. Every domain table has `workspace_id` and a row-level-security policy
(`workspace_id = app_workspace()`), so a workspace cannot read, change, delete or smuggle rows of another,
even through a query that forgot a filter.

- **How it binds:** the API logs in as the owner role (`QUESTS_DATABASE_URL`) and runs `SET ROLE quests_app`
  on every connection (no `BYPASSRLS`, not the table owner). Before each statement the Go driver wrapper
  sets `app.workspace_id` from the request context; `INSERT`s get `workspace_id` from the same setting by
  default. No workspace on the context = no rows (fail closed). `pg_dump` and migrations use the login role
  and are not affected.
- **Who is in which workspace:** `workspace` + `workspace_member`. A login session works in the workspace
  it is pinned to (switch: Settings → Пространства, `POST /api/workspaces/{id}/switch`); an API token is pinned
  when minted, so the Telegram bot, MCP, CLI and HUDs need no changes.
  `quests-server token add <name> [user] --workspace <id>`.
- **CLI / scripts:** `QUESTS_WORKSPACE=<id>` picks a workspace per call (checked against membership; the
  in-process token used by check commands may use any — each check command gets its quest's workspace in
  that variable).
- **Background work** (expiry, window notices, template emit, auto-checks, attachment backup listing) iterates
  every workspace under that workspace's context. Events carry the workspace and are delivered only to its peers.
- **Runners** belong to a workspace; their tokens are resolved through SECURITY DEFINER functions.
  `QUESTS_WORKSPACE=<id> quests-server runner add <label>`.
- **Accounts:** `quests-server useradd` gives the first account workspace 1 (the migrated data) and every
  later one a new personal workspace with the default categories. `quests-server workspace ls|add|member`.
- **Members and invitations:** roles `owner` / `member` (members get full data access, owners also manage
  people). Settings → Пространства lists members (owners remove anyone, anyone leaves; the last owner cannot)
  and mints a one-time invitation link (`/?invite=<token>`, 7 days, only its hash is stored). Opening it:
  a logged-in account joins; otherwise the page signs up a new account (login + password) that joins. This is
  the only self-service signup. An account left in no workspace stays signed in and is offered to create one.
  API: `GET/DELETE /api/workspaces/{id}/members`, `POST /api/workspaces/{id}/invites`,
  `GET|POST /api/invites/{token}[/accept]`.
- Category slugs, tag slugs and runner labels are unique per workspace.

Tests: `QUESTS_TEST_PG_URL=… go test ./go/internal/db` includes `TestPGWorkspaceIsolation` (needs a login
role that may `CREATE ROLE`).
