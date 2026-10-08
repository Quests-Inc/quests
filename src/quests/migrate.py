"""Run Alembic migrations (sync SQLite)."""

from __future__ import annotations

from alembic import command
from alembic.config import Config
from sqlalchemy import create_engine, inspect, text

from quests.config import DATABASE_URL_SYNC, DATA_DIR, DB_PATH, IS_POSTGRES, ROOT

ALEMBIC_INI = ROOT / "alembic.ini"


def alembic_config() -> Config:
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    cfg = Config(str(ALEMBIC_INI))
    cfg.set_main_option("sqlalchemy.url", DATABASE_URL_SYNC)
    cfg.set_main_option("script_location", str(ROOT / "alembic"))
    # PostgreSQL has its own chain: a baseline of the SQLite one, then PG-only revisions.
    versions = "versions_pg" if IS_POSTGRES else "versions"
    cfg.set_main_option("version_locations", str(ROOT / "alembic" / versions))
    return cfg


def _sync_engine():
    kwargs: dict = {}
    if DATABASE_URL_SYNC.startswith("sqlite"):
        kwargs["connect_args"] = {"timeout": 30}
    return create_engine(DATABASE_URL_SYNC, **kwargs)


def _has_table(name: str) -> bool:
    if not IS_POSTGRES and not DB_PATH.exists():
        return False
    engine = _sync_engine()
    try:
        return name in inspect(engine).get_table_names()
    finally:
        engine.dispose()


def current_revision() -> str | None:
    if not _has_table("alembic_version"):
        return None
    engine = _sync_engine()
    try:
        with engine.connect() as conn:
            row = conn.execute(text("SELECT version_num FROM alembic_version")).first()
            return str(row[0]) if row else None
    finally:
        engine.dispose()


def upgrade_to_head() -> str:
    """Apply migrations. Stamp legacy DBs that already have tables via create_all."""
    cfg = alembic_config()
    has_quest = _has_table("quest")
    rev = current_revision()

    if has_quest and rev is None:
        # Pre-Alembic DB (or empty alembic_version): keep data, mark as head.
        command.stamp(cfg, "head")
        return "stamped legacy database to head"

    command.upgrade(cfg, "head")
    return "upgraded to head"


def main() -> None:
    import argparse

    parser = argparse.ArgumentParser(description="Quests database migrations")
    parser.add_argument(
        "action",
        nargs="?",
        default="upgrade",
        choices=("upgrade", "current", "stamp-head"),
    )
    args = parser.parse_args()
    if args.action == "upgrade":
        print(upgrade_to_head())
        print("revision:", current_revision())
    elif args.action == "current":
        print(current_revision() or "(none)")
    elif args.action == "stamp-head":
        command.stamp(alembic_config(), "head")
        print("stamped to head:", current_revision())


if __name__ == "__main__":
    main()
