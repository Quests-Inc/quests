"""PostgreSQL baseline (equivalent to the SQLite chain in alembic/versions up to 1a2b3c4d5e6f)

PostgreSQL databases use this directory instead of alembic/versions (see
quests.migrate.alembic_config). New migrations after the cutover go here,
written for PostgreSQL only.
"""

from pathlib import Path

from alembic import op

revision = "1a2b3c4d5e6f"
down_revision = None
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("baseline.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8"))


def downgrade() -> None:
    raise NotImplementedError("baseline cannot be downgraded")
