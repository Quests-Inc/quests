"""user_totp / user_recovery_code: two-factor sign-in (PostgreSQL only)"""

from pathlib import Path

from alembic import op

revision = "b6d8f0a2c4e1"
down_revision = "a4c6e8b0d2f1"
branch_labels = None
depends_on = None

_SQL = Path(__file__).with_name("b6d8f0a2c4e1_user_totp.sql")


def upgrade() -> None:
    op.get_bind().exec_driver_sql(_SQL.read_text(encoding="utf-8").replace("%", "%%"))


def downgrade() -> None:
    raise NotImplementedError
