"""drop hero sheet, attributes, metric ledger and reward_attrs (gamification removed)

Revision ID: b1c2d3e4f5a6
Revises: a0b1c2d3e4f5
Create Date: 2026-10-05 20:30:00.000000

"""

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "b1c2d3e4f5a6"
down_revision: Union[str, Sequence[str], None] = "a0b1c2d3e4f5"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    with op.batch_alter_table("metricledger", schema=None) as batch_op:
        batch_op.drop_index(batch_op.f("ix_metricledger_reason"))
        batch_op.drop_index(batch_op.f("ix_metricledger_quest_id"))
        batch_op.drop_index(batch_op.f("ix_metricledger_kind"))
        batch_op.drop_index(batch_op.f("ix_metricledger_at"))
    op.drop_table("metricledger")

    with op.batch_alter_table("heroattribute", schema=None) as batch_op:
        batch_op.drop_index(batch_op.f("ix_heroattribute_attr_id"))
    op.drop_table("heroattribute")
    op.drop_table("herosheet")

    with op.batch_alter_table("questtemplate", schema=None) as batch_op:
        batch_op.drop_column("reward_attrs")
    with op.batch_alter_table("quest", schema=None) as batch_op:
        batch_op.drop_column("reward_attrs")


def downgrade() -> None:
    with op.batch_alter_table("quest", schema=None) as batch_op:
        batch_op.add_column(
            sa.Column("reward_attrs", sa.String(length=500), nullable=True)
        )
    with op.batch_alter_table("questtemplate", schema=None) as batch_op:
        batch_op.add_column(
            sa.Column("reward_attrs", sa.String(length=500), nullable=True)
        )

    op.create_table(
        "herosheet",
        sa.Column("id", sa.Integer(), nullable=False),
        sa.Column("xp", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("momentum", sa.Integer(), nullable=False, server_default="50"),
        sa.Column("momentum_updated_at", sa.DateTime(), nullable=False),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_table(
        "heroattribute",
        sa.Column("id", sa.Integer(), nullable=False),
        sa.Column("attr_id", sa.String(length=8), nullable=False),
        sa.Column("rank", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("progress", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("updated_at", sa.DateTime(), nullable=False),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("attr_id", name="uq_hero_attribute_attr_id"),
    )
    with op.batch_alter_table("heroattribute", schema=None) as batch_op:
        batch_op.create_index(
            batch_op.f("ix_heroattribute_attr_id"), ["attr_id"], unique=False
        )

    op.create_table(
        "metricledger",
        sa.Column("id", sa.Integer(), nullable=False),
        sa.Column("at", sa.DateTime(), nullable=False),
        sa.Column("kind", sa.String(length=16), nullable=False),
        sa.Column("attr_id", sa.String(length=8), nullable=True),
        sa.Column("delta", sa.Integer(), nullable=False),
        sa.Column("balance_after", sa.Integer(), nullable=False),
        sa.Column("quest_id", sa.Integer(), nullable=True),
        sa.Column("reason", sa.String(length=64), nullable=False),
        sa.Column("flavor", sa.String(length=300), nullable=True),
        sa.ForeignKeyConstraint(["quest_id"], ["quest.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "quest_id", "reason", name="uq_metric_ledger_quest_reason"
        ),
    )
    with op.batch_alter_table("metricledger", schema=None) as batch_op:
        batch_op.create_index(batch_op.f("ix_metricledger_at"), ["at"], unique=False)
        batch_op.create_index(
            batch_op.f("ix_metricledger_kind"), ["kind"], unique=False
        )
        batch_op.create_index(
            batch_op.f("ix_metricledger_quest_id"), ["quest_id"], unique=False
        )
        batch_op.create_index(
            batch_op.f("ix_metricledger_reason"), ["reason"], unique=False
        )
