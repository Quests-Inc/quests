"""workspace invitation links (PostgreSQL only; see the .sql next to this file)"""

from pathlib import Path

from alembic import op

revision = "d8f2b4a6c1e3"
down_revision = "c7e1a9b3d5f2"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("d8f2b4a6c1e3_workspace_invites.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
