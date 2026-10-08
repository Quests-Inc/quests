"""workspace.trusted: gate on server-side command execution (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "e9a3c5b7d1f4"
down_revision = "d8f2b4a6c1e3"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("e9a3c5b7d1f4_workspace_trusted.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
