#!/usr/bin/env python3
"""Copy a Quests SQLite database into an (already migrated) PostgreSQL one.

    uv run python scripts/sqlite_to_pg.py data/quests.db postgresql://user@host/db [--force]

The target must have been migrated first (any PG revision; copied rows go to workspace 1) (`QUESTS_DATABASE_URL=… quests-server migrate`)
and be empty (--force truncates it). Tables are copied in foreign-key order, serial
sequences are advanced past the copied ids, and row counts are compared at the end.
Everything runs in one transaction: any failure leaves the target untouched.
"""

from __future__ import annotations

import argparse
import sqlite3
import sys

import psycopg

# workspace/workspace_member are created (and seeded with workspace 1) by the PG migrations.
SKIP = {"alembic_version", "workspace", "workspace_member"}
SQLITE_REV = "1a2b3c4d5e6f"


def fk_order(cur: psycopg.Cursor) -> list[str]:
    cur.execute("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
    tables = {r[0] for r in cur.fetchall()} - SKIP
    cur.execute(
        """
        SELECT c.conrelid::regclass::text, c.confrelid::regclass::text
        FROM pg_constraint c WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace
        """
    )
    deps: dict[str, set[str]] = {t: set() for t in tables}
    for child, parent in cur.fetchall():
        if child != parent and child in deps and parent in tables:
            deps[child].add(parent)
    order: list[str] = []
    while deps:
        ready = sorted(t for t, d in deps.items() if not d)
        if not ready:
            raise SystemExit(f"foreign-key cycle among {sorted(deps)}")
        for t in ready:
            order.append(t)
            del deps[t]
        for d in deps.values():
            d.difference_update(ready)
    return order


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    ap.add_argument("sqlite_path")
    ap.add_argument("pg_url")
    ap.add_argument("--force", action="store_true", help="truncate target tables first")
    args = ap.parse_args()

    src = sqlite3.connect(f"file:{args.sqlite_path}?mode=ro", uri=True)
    src.row_factory = sqlite3.Row
    src_tables = {r[0] for r in src.execute("SELECT name FROM sqlite_master WHERE type='table'")}

    with psycopg.connect(args.pg_url) as pg, pg.cursor() as cur:
        order = fk_order(cur)
        missing = src_tables - set(order) - SKIP - {"sqlite_sequence"}
        if missing:
            print(f"WARNING: SQLite tables with no PG counterpart (skipped): {sorted(missing)}")

        src_rev = src.execute("SELECT version_num FROM alembic_version").fetchone()
        if not src_rev or src_rev[0] != SQLITE_REV:
            raise SystemExit(f"sqlite revision {src_rev and src_rev[0]} != {SQLITE_REV}: migrate the SQLite DB first")
        # Rows copied without an explicit workspace_id land in workspace 1.
        cur.execute("SELECT set_config('app.workspace_id', '1', false)")

        if args.force:
            cur.execute("TRUNCATE " + ", ".join(order) + " RESTART IDENTITY CASCADE")
        else:
            for t in order:
                cur.execute(f"SELECT 1 FROM {t} LIMIT 1")
                if cur.fetchone():
                    raise SystemExit(f"target table {t} is not empty (use --force to truncate)")

        ok = True
        for t in order:
            if t not in src_tables:
                print(f"{t:24} (absent in sqlite)")
                continue
            cur.execute(
                "SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=%s",
                (t,),
            )
            pg_cols = {r[0] for r in cur.fetchall()}
            src_cols = [r[1] for r in src.execute(f'PRAGMA table_info("{t}")')]
            cols = [c for c in src_cols if c in pg_cols]
            dropped = set(src_cols) - pg_cols
            if dropped:
                print(f"WARNING: {t}: columns not in PG dropped: {sorted(dropped)}")
            rows = [tuple(r[c] for c in cols) for r in src.execute(f'SELECT {", ".join(chr(34)+c+chr(34) for c in cols)} FROM "{t}"')]
            if rows:
                q = f'INSERT INTO {t} ({", ".join(chr(34)+c+chr(34) for c in cols)}) VALUES ({", ".join(["%s"] * len(cols))})'
                cur.executemany(q, rows)
            if "id" in pg_cols:
                cur.execute(
                    f"SELECT setval(pg_get_serial_sequence('{t}', 'id'), COALESCE((SELECT MAX(id) FROM {t}), 0) + 1, false)"
                )
            cur.execute(f"SELECT COUNT(*) FROM {t}")
            n_pg = cur.fetchone()[0]
            n_src = src.execute(f'SELECT COUNT(*) FROM "{t}"').fetchone()[0]
            flag = "" if n_pg == n_src else "   <-- MISMATCH"
            ok = ok and n_pg == n_src
            print(f"{t:24} sqlite={n_src:6} pg={n_pg:6}{flag}")
        if ok:
            # Existing accounts own the migrated data: members of workspace 1.
            cur.execute(
                "INSERT INTO workspace_member (workspace_id, user_id, role) "
                "SELECT 1, id, 'owner' FROM appuser ON CONFLICT DO NOTHING"
            )
            print(f"workspace 1 members added: {cur.rowcount}")
        if not ok:
            pg.rollback()
            print("row counts differ; rolled back", file=sys.stderr)
            return 1
        pg.commit()
    print("done")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
