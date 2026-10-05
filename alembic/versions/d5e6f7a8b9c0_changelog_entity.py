"""questchangelog: entity_type/entity_id/parent for notes and attachments

Revision ID: d5e6f7a8b9c0
Revises: b1c2d3e4f5a6
Create Date: 2026-10-05 21:00:00.000000

"""

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "d5e6f7a8b9c0"
down_revision: Union[str, Sequence[str], None] = "b1c2d3e4f5a6"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    with op.batch_alter_table("questchangelog", schema=None) as batch_op:
        batch_op.add_column(
            sa.Column(
                "entity_type",
                sa.String(length=16),
                nullable=False,
                server_default="quest",
            )
        )
        batch_op.add_column(sa.Column("entity_id", sa.Integer(), nullable=True))
        batch_op.add_column(sa.Column("parent_type", sa.String(length=16), nullable=True))
        batch_op.add_column(sa.Column("parent_id", sa.Integer(), nullable=True))
        batch_op.create_index(
            "ix_questchangelog_entity", ["entity_type", "entity_id"], unique=False
        )
        batch_op.create_index(
            "ix_questchangelog_parent", ["parent_type", "parent_id"], unique=False
        )
    op.execute("UPDATE questchangelog SET entity_id = quest_id WHERE quest_id IS NOT NULL")


def downgrade() -> None:
    with op.batch_alter_table("questchangelog", schema=None) as batch_op:
        batch_op.drop_index("ix_questchangelog_parent")
        batch_op.drop_index("ix_questchangelog_entity")
        batch_op.drop_column("parent_id")
        batch_op.drop_column("parent_type")
        batch_op.drop_column("entity_id")
        batch_op.drop_column("entity_type")
