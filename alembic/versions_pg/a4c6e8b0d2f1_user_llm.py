"""user_llm: per-account LLM provider + encrypted API key (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "a4c6e8b0d2f1"
down_revision = "f1b4d6c8e2a5"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("a4c6e8b0d2f1_user_llm.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
