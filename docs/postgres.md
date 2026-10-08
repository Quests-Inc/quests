# PostgreSQL

The API runs on PostgreSQL when `QUESTS_DATABASE_URL` (`postgresql://user:pw@host:port/db`) is set,
and on the SQLite file in `QUESTS_DATA_DIR` otherwise. Both `quests-migrate` and `quests-server` read it.

## Schema chains

| DB | Migrations | Notes |
|---|---|---|
| SQLite | `alembic/versions/` | legacy chain, frozen after cutover |
| PostgreSQL | `alembic/versions_pg/` | `1a2b3c4d5e6f` is a baseline equal to the SQLite head; new revisions go here, PG-only |

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
3. Migrate PG and copy the data (from a host with the volume and uv; `QUESTS_DATABASE_URL` points at the new DB):
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
