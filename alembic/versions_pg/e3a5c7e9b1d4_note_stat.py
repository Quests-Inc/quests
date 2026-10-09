"""note_stat: decaying reading signals per note (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "e3a5c7e9b1d4"
down_revision = "d1f3a5c7e9b2"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("e3a5c7e9b1d4_note_stat.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
