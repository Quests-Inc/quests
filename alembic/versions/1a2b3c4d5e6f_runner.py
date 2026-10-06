"""runners and their job queue (checks executed on another device)

Revision ID: 1a2b3c4d5e6f
Revises: e6f7a8b9c0d1
Create Date: 2026-10-06 12:00:00.000000

"""

from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

revision: str = "1a2b3c4d5e6f"
down_revision: Union[str, Sequence[str], None] = "e6f7a8b9c0d1"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Which runner executes a step's check (empty = the server itself).
    with op.batch_alter_table("queststep", schema=None) as batch_op:
        batch_op.add_column(sa.Column("run_on", sa.String(length=64), nullable=True))
    with op.batch_alter_table("questtemplatestep", schema=None) as batch_op:
        batch_op.add_column(sa.Column("run_on", sa.String(length=64), nullable=True))
    # ...and which runner executes a template's emit_pool_command.
    with op.batch_alter_table("questtemplate", schema=None) as batch_op:
        batch_op.add_column(sa.Column("emit_run_on", sa.String(length=64), nullable=True))
    op.create_table(
        "runner",
        sa.Column("id", sa.Integer(), nullable=False),
        sa.Column("label", sa.String(length=64), nullable=False),
        sa.Column("reg_hash", sa.String(length=64), nullable=True),
        sa.Column("reg_expires_at", sa.DateTime(), nullable=True),
        sa.Column("token_hash", sa.String(length=64), nullable=True),
        sa.Column("hostname", sa.String(length=128), nullable=True),
        sa.Column("version", sa.String(length=32), nullable=True),
        sa.Column("last_seen_at", sa.DateTime(), nullable=True),
        sa.Column("revoked", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("label"),
    )
    op.create_table(
        "runner_job",
        sa.Column("id", sa.Integer(), nullable=False),
        sa.Column("runner_label", sa.String(length=64), nullable=False),
        sa.Column("kind", sa.String(length=16), nullable=False),
        sa.Column("quest_id", sa.Integer(), nullable=True),
        sa.Column("step_id", sa.Integer(), nullable=True),
        sa.Column("template_id", sa.Integer(), nullable=True),
        sa.Column("title", sa.String(length=200), nullable=False, server_default=""),
        sa.Column("dedupe_key", sa.String(length=128), nullable=True),
        sa.Column("command", sa.Text(), nullable=False),
        sa.Column("state", sa.String(length=12), nullable=False, server_default="queued"),
        sa.Column("attempt_no", sa.Integer(), nullable=False, server_default="1"),
        sa.Column("timeout_seconds", sa.Integer(), nullable=False),
        sa.Column("lease_id", sa.String(length=36), nullable=True),
        sa.Column("lease_until", sa.DateTime(), nullable=True),
        sa.Column("valid_until", sa.DateTime(), nullable=False),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("assigned_at", sa.DateTime(), nullable=True),
        sa.Column("started_at", sa.DateTime(), nullable=True),
        sa.Column("finished_at", sa.DateTime(), nullable=True),
        sa.Column("exit_code", sa.Integer(), nullable=True),
        sa.Column("timed_out", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.Column("stdout", sa.Text(), nullable=False, server_default=""),
        sa.Column("stderr", sa.Text(), nullable=False, server_default=""),
        sa.Column("progress_current", sa.Integer(), nullable=True),
        sa.Column("progress_total", sa.Integer(), nullable=True),
        sa.Column("log_tail", sa.Text(), nullable=False, server_default=""),
        sa.Column("reason", sa.Text(), nullable=False, server_default=""),
        sa.Column("cancel", sa.Boolean(), nullable=False, server_default=sa.false()),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_runner_job_label_state", "runner_job", ["runner_label", "state"])
    # One unfinished job per dedupe key: the queue does not grow while a runner is away.
    op.create_index(
        "ux_runner_job_open_dedupe",
        "runner_job",
        ["dedupe_key"],
        unique=True,
        sqlite_where=sa.text("dedupe_key IS NOT NULL AND state IN ('queued','assigned','running')"),
    )


def downgrade() -> None:
    with op.batch_alter_table("questtemplate", schema=None) as batch_op:
        batch_op.drop_column("emit_run_on")
    with op.batch_alter_table("questtemplatestep", schema=None) as batch_op:
        batch_op.drop_column("run_on")
    with op.batch_alter_table("queststep", schema=None) as batch_op:
        batch_op.drop_column("run_on")
    op.drop_index("ux_runner_job_open_dedupe", table_name="runner_job")
    op.drop_index("ix_runner_job_label_state", table_name="runner_job")
    op.drop_table("runner_job")
    op.drop_table("runner")
