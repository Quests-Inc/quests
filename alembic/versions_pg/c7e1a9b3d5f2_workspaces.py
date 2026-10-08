"""workspaces: workspace_id + row-level security on every domain table

PostgreSQL only. See the .sql next to this file for the design notes.
"""

from pathlib import Path

from alembic import op

revision = "c7e1a9b3d5f2"
down_revision = "1a2b3c4d5e6f"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("c7e1a9b3d5f2_workspaces.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))  # psycopg placeholder escaping


def downgrade() -> None:
    raise NotImplementedError("workspaces cannot be downgraded")
