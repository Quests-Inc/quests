"""note check: check_* columns on note, runner_job.note_id, secret.note_id (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "f5b7d9e1a3c6"
down_revision = "e3a5c7e9b1d4"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("f5b7d9e1a3c6_note_check.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
