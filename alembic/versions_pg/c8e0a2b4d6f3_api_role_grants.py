"""api role grants: the API role cannot change workspace.trusted, passwords or the schema revision (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "c8e0a2b4d6f3"
down_revision = "b6d8f0a2c4e1"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("c8e0a2b4d6f3_api_role_grants.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
