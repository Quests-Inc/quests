# PostgreSQL

The API runs on PostgreSQL when `QUESTS_DATABASE_URL` (`postgresql://user:pw@host:port/db`) is set,
and on the SQLite file in `QUESTS_DATA_DIR` otherwise. `quests-server migrate` (run by the api image's entrypoint before the server starts) and `quests-server` both read it.

## Schema migrations

Migrations are plain SQL files embedded in the binary and applied by `quests-server migrate`
(`go/internal/db/migrate.go`):

| DB | Files | Notes |
|---|---|---|
| PostgreSQL | `go/internal/db/sql/pg/NNNN_*.sql` | the chain that grows; each file runs in its own transaction under an advisory lock |
| SQLite | `go/internal/db/sql/sqlite/` | a single baseline (frozen chain head `1a2b3c4d5e6f`); SQLite is for local development |

The applied revision is kept in `alembic_version` (same table and format Alembic used), so databases
migrated by Alembic continue from where they are. Files `0001`–`0011` carry their Alembic revision id
(`NNNN_<12 hex>_<name>.sql`); new ones are `NNNN_<name>.sql` and their revision is just `NNNN`.

**Adding a migration:** create `go/internal/db/sql/pg/NNNN_<name>.sql` with the next number (PG only, no
Python wrapper, no list to update — the head is derived from the files). The API refuses to start on an older
schema and warns on a newer one; run `quests-server migrate current` to see the revision.

The table is still called `alembic_version` (and files `0001`–`0011` carry Alembic ids) because the production
database was created by Alembic; the Alembic chain itself has been removed from the repository (see git history).

Baseline conventions (kept so the Go store works unchanged on both):
boolean-ish columns are `INTEGER` 0/1, timestamps are naive-UTC `TIMESTAMP`, `VARCHAR(n)` is `TEXT`
(prod data already exceeds the declared lengths).

## Local

```bash
export QUESTS_DATABASE_URL=postgresql://quests@127.0.0.1:5432/quests
quests-server migrate
./scripts/run-server.sh
```

Go tests that need a real PG (`go/internal/db`, applies the baseline into a throwaway schema):

```bash
QUESTS_TEST_PG_URL=postgresql://quests@127.0.0.1:5432/quests go test ./go/internal/db
```

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
- **Server-side commands are the operator's privilege (quest=348).** `check_command` without `run_on` and
  `emit_pool_command` run `sh -c` inside the API container; in an untrusted workspace that would be a shell on
  the server. So only trusted workspaces may use them: saving such a command is refused (403), and anything
  already stored is not executed (checks, emit pools); runners (`run_on`) work everywhere. The flag is host-only:
  `quests-server workspace trust|untrust <id>` (workspace 1 is trusted, new ones are not); there is no API or UI
  for it on purpose. The UI hides «на сервере» in untrusted workspaces. Commands run with the server's database
  credentials (`QUESTS_DATABASE_URL`, `POSTGRES_*`, `PG*`) removed from their environment; the rest of the root
  `.env` still passes through for the operator's own scripts.
- **Rename / delete (quest=347):** owners rename and delete from Settings → Пространства (deleting needs the
  exact name typed). A deleted workspace disappears at once — its tokens, runners and sessions stop (a token
  never falls back to another workspace; a session moves to the user's next one) — and is purged after 7 days
  (files on WebDAV first, then the rows; cascades over every table). `quests-server workspace restore <id>`
  undoes it before that; `workspace ls` lists pending purges. Trusted workspaces cannot be deleted until
  `workspace untrust <id>`.
- **Export / import (quest=349):** Settings → Пространства → Архив. Export (owners) streams a zip:
  `manifest.json`, `tables/<table>.jsonl` (quests, steps, questlines, notes, templates, tags, categories,
  attachments, change log, emit state; secrets only with `?secrets=1`), `files/` (attachment bytes from WebDAV)
  and `icons/`. Import (any account, needs a ClamAV-scanned upload path for files) always creates a **new**
  workspace owned by the importer and renumbers everything: foreign keys go through old→new id maps and the
  `kind=ID` references in text are rewritten with the same maps. A reference to something that is not in the
  archive becomes `kind#ID` (otherwise it could start pointing at an unrelated entity that carries that id in the
  new workspace). Commands that would run on the server are dropped (the new workspace is untrusted), runner
  commands are kept. The import is one transaction: a bad archive (unknown column, wrong version, infected file)
  leaves nothing behind, neither rows nor files.
- Category slugs, tag slugs and runner labels are unique per workspace.

Tests: `QUESTS_TEST_PG_URL=… go test ./go/internal/db` includes `TestPGWorkspaceIsolation` (needs a login
role that may `CREATE ROLE`).
