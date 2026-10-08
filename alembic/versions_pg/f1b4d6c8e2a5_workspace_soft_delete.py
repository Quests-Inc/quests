"""workspace.deleted_at: soft deletion with a purge grace period (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "f1b4d6c8e2a5"
down_revision = "e9a3c5b7d1f4"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("f1b4d6c8e2a5_workspace_soft_delete.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
