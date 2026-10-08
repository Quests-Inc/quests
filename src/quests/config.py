from __future__ import annotations

import os
from pathlib import Path

# Project root: …/Quests (editable install) or QUESTS_ROOT in containers.
_ROOT_ENV = (os.environ.get("QUESTS_ROOT") or "").strip()
ROOT = Path(_ROOT_ENV) if _ROOT_ENV else Path(__file__).resolve().parents[2]

_DATA_ENV = (os.environ.get("QUESTS_DATA_DIR") or "").strip()
DATA_DIR = Path(_DATA_ENV) if _DATA_ENV else ROOT / "data"
DB_PATH = DATA_DIR / "quests.db"
# QUESTS_DATABASE_URL (postgresql://user:pw@host:port/db) switches the whole stack
# to PostgreSQL; unset keeps the local SQLite file.
_PG_URL = (os.environ.get("QUESTS_DATABASE_URL") or "").strip()
if _PG_URL.startswith("postgres://"):
    _PG_URL = "postgresql://" + _PG_URL[len("postgres://") :]
IS_POSTGRES = bool(_PG_URL)
DATABASE_URL = f"sqlite+aiosqlite:///{DB_PATH}"
# Sync URL for Alembic (and any sync tooling).
DATABASE_URL_SYNC = (
    _PG_URL.replace("postgresql://", "postgresql+psycopg://", 1)
    if IS_POSTGRES
    else f"sqlite:///{DB_PATH}"
)

# Bind address. Use 0.0.0.0 (or a LAN IP) when the API must accept remote HUD/bot/web.
HOST = (os.environ.get("QUESTS_HOST") or "127.0.0.1").strip() or "127.0.0.1"
PORT = int(os.environ.get("QUESTS_PORT") or "8765")
# Hot-reload only for local dev (never in Docker/prod).
RELOAD = (os.environ.get("QUESTS_RELOAD") or "").strip().lower() in {
    "1",
    "true",
    "yes",
    "on",
}

# Comma-separated CORS origins. Empty → Vite localhost defaults.
# Example remote: QUESTS_CORS_ORIGINS=https://quests.example.com,http://192.168.1.100:5173
_cors_raw = (os.environ.get("QUESTS_CORS_ORIGINS") or "").strip()
if _cors_raw:
    CORS_ORIGINS = [o.strip() for o in _cors_raw.split(",") if o.strip()]
else:
    CORS_ORIGINS = [
        "http://127.0.0.1:5173",
        "http://localhost:5173",
    ]
