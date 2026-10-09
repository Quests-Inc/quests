"""note.search_tsv + pg_trgm: ranked full-text search over notes (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "d1f3a5c7e9b2"
down_revision = "c8e0a2b4d6f3"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("d1f3a5c7e9b2_note_search.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
