from datetime import datetime, timezone
from enum import Enum
from typing import List, Optional

from pydantic import field_serializer
from sqlalchemy import UniqueConstraint
from sqlmodel import Field, Relationship, SQLModel

from quests.timeutil import to_db_utc, to_utc_iso


def utcnow() -> datetime:
    """Naive UTC for DB columns (SQLite has no timezone)."""
    now = to_db_utc(datetime.now(timezone.utc))
    assert now is not None
    return now


class QuestStatus(str, Enum):
    """Lifecycle status."""

    active = "active"
    expired = "expired"
    frozen = "frozen"  # manual "заморожено" — parked by the user, not auto-set
    completed = "completed"
    failed = "failed"
    archived = "archived"


class TemplateFreq(str, Enum):
    daily = "daily"
    weekly = "weekly"


class TemplateEmitMode(str, Enum):
    """How a due template becomes a quest instance."""

    fixed = "fixed"  # appear at start of period (current behaviour)
    surprise = "surprise"  # chance + random time within window


class TemplateEmitOutcome(str, Enum):
    miss = "miss"
    scheduled = "scheduled"
    materialized = "materialized"
    error = "error"  # emit_pool_command exhausted its retries this period


class QuestSignificance(str, Enum):
    """Game-style rarity (not priority)."""

    insignificant = "insignificant"  # незначительное
    common = "common"  # обычное
    uncommon = "uncommon"  # необычное
    epic = "epic"  # эпическое
    legendary = "legendary"  # легендарное


SIGNIFICANCE_LABEL_RU: dict[str, str] = {
    QuestSignificance.insignificant.value: "незначительное",
    QuestSignificance.common.value: "обычное",
    QuestSignificance.uncommon.value: "необычное",
    QuestSignificance.epic.value: "эпическое",
    QuestSignificance.legendary.value: "легендарное",
}


# Seed catalog (slug, label, sort_order).
# slug, label, sort_order, color (hex)
CATEGORY_SEED: list[tuple[str, str, int, str]] = [
    ("work", "Работа", 10, "#5a8a9a"),
    ("routine", "Рутина", 20, "#8a8578"),
    ("health", "Здоровье", 30, "#7a9e3a"),
    ("study", "Учёба", 40, "#6a7ab8"),
    ("fun", "Развлечения", 50, "#c47a20"),
]


class QuestCategory(SQLModel, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    slug: str = Field(max_length=32, unique=True, index=True)
    label: str = Field(max_length=64)
    sort_order: int = Field(default=0)
    # Accent hex for UI (sidebar / slider), e.g. "#5a8a9a".
    color: str = Field(default="#9a9a9a", max_length=16)
    created_at: datetime = Field(default_factory=utcnow)


class QuestCategoryRead(SQLModel):
    id: int
    slug: str
    label: str
    sort_order: int
    color: str


class QuestBase(SQLModel):
    title: str = Field(min_length=1, max_length=200)
    description: str = ""
    status: QuestStatus = QuestStatus.active
    significance: QuestSignificance = QuestSignificance.common
    pinned: bool = False
    sort_order: int = 0
    # Due instant (UTC). Null = no deadline.
    deadline_at: Optional[datetime] = None
    # Length of the countdown window ending at deadline_at (seconds).
    duration_seconds: Optional[int] = Field(default=None, ge=1)
    category_id: Optional[int] = Field(
        default=None, foreign_key="questcategory.id", index=True
    )
    questline_id: Optional[int] = Field(
        default=None, foreign_key="questline.id", index=True
    )
    # Soften created/appeared/started overlay toasts for background pipelines.
    automated: bool = False


class QuestLineBase(SQLModel):
    title: str = Field(min_length=1, max_length=200)
    description: str = ""
    category_id: Optional[int] = Field(
        default=None, foreign_key="questcategory.id", index=True
    )
    color: str = Field(default="#9a9a9a", max_length=16)
    icon: str = Field(default="document", max_length=32)


class QuestLine(QuestLineBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    created_at: datetime = Field(default_factory=utcnow)
    updated_at: datetime = Field(default_factory=utcnow)
    # Uploaded image filename under data/questline-icons/ (e.g. "3.png").
    custom_icon: Optional[str] = Field(default=None, max_length=64)
    category: Optional[QuestCategory] = Relationship()


class QuestLineCreate(QuestLineBase):
    pass


class QuestLineUpdate(SQLModel):
    title: Optional[str] = Field(default=None, min_length=1, max_length=200)
    description: Optional[str] = None
    category_id: Optional[int] = None
    color: Optional[str] = Field(default=None, max_length=16)
    icon: Optional[str] = Field(default=None, max_length=32)


class QuestLineRead(QuestLineBase):
    id: int
    created_at: datetime
    updated_at: datetime
    custom_icon: Optional[str] = None
    category_slug: Optional[str] = None
    category_label: Optional[str] = None
    category_color: Optional[str] = None
    # Present when a custom upload is set; clients prefer this over built-in `icon`.
    icon_url: Optional[str] = None

    @field_serializer("created_at", "updated_at", when_used="json")
    def _ser_utc(self, value: Optional[datetime]) -> Optional[str]:
        return to_utc_iso(value)


class Quest(QuestBase, table=True):
    __table_args__ = (
        UniqueConstraint("template_id", "period_key", name="uq_quest_template_period"),
    )

    id: Optional[int] = Field(default=None, primary_key=True)
    created_at: datetime = Field(default_factory=utcnow)
    updated_at: datetime = Field(default_factory=utcnow)
    completed_at: Optional[datetime] = None
    template_id: Optional[int] = Field(
        default=None, foreign_key="questtemplate.id", index=True
    )
    period_key: Optional[str] = Field(default=None, max_length=32, index=True)
    # How the quest was created: 'template:<id>' or the client (web/cli/mcp/…).
    source: Optional[str] = Field(default=None, max_length=48)
    steps: List["QuestStep"] = Relationship(
        back_populates="quest",
        sa_relationship_kwargs={
            "cascade": "all, delete-orphan",
            "order_by": "QuestStep.sort_order",
        },
    )
    category: Optional[QuestCategory] = Relationship()
    questline: Optional[QuestLine] = Relationship()


class QuestStepBase(SQLModel):
    title: str = Field(min_length=1, max_length=200)
    description: str = ""
    # Binary step: total=1, current 0|1. Quantified: e.g. 5/8 herbs.
    progress_current: int = Field(default=0, ge=0)
    progress_total: int = Field(default=1, ge=1)
    sort_order: int = 0
    # Optional shell check: stdout → progress_current, polled every N seconds.
    check_command: Optional[str] = Field(default=None, max_length=2000)
    check_interval_seconds: Optional[int] = Field(default=None, ge=15)
    wait_previous: bool = False
    run_mode: str = Field(default="poll", max_length=16)


class QuestStep(QuestStepBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    quest_id: Optional[int] = Field(default=None, foreign_key="quest.id", index=True)
    quest: Optional[Quest] = Relationship(back_populates="steps")
    # Last successful/attempted auto-check (UTC naive).
    check_last_run_at: Optional[datetime] = None
    run_status: Optional[str] = Field(default=None, max_length=16)

    @property
    def done(self) -> bool:
        return self.progress_current >= self.progress_total


class QuestStepCreate(QuestStepBase):
    pass


class QuestStepUpdate(SQLModel):
    title: Optional[str] = Field(default=None, min_length=1, max_length=200)
    description: Optional[str] = None
    progress_current: Optional[int] = Field(default=None, ge=0)
    progress_total: Optional[int] = Field(default=None, ge=1)
    sort_order: Optional[int] = None
    check_command: Optional[str] = Field(default=None, max_length=2000)
    check_interval_seconds: Optional[int] = Field(default=None, ge=15)
    wait_previous: Optional[bool] = None
    run_mode: Optional[str] = Field(default=None, max_length=16)


class QuestStepRead(QuestStepBase):
    id: int
    quest_id: int
    done: bool = False
    check_last_run_at: Optional[datetime] = None
    run_status: Optional[str] = None

    @field_serializer("check_last_run_at", when_used="json")
    def _ser_check_last(self, value: Optional[datetime]) -> Optional[str]:
        return to_utc_iso(value)


class QuestCreate(QuestBase):
    steps: List[QuestStepCreate] = Field(default_factory=list)


class QuestUpdate(SQLModel):
    """Partial quest update. Steps are not replaced here — use step CRUD routes."""

    title: Optional[str] = Field(default=None, min_length=1, max_length=200)
    description: Optional[str] = None
    status: Optional[QuestStatus] = None
    significance: Optional[QuestSignificance] = None
    pinned: Optional[bool] = None
    sort_order: Optional[int] = None
    deadline_at: Optional[datetime] = None
    duration_seconds: Optional[int] = Field(default=None, ge=1)
    category_id: Optional[int] = None
    questline_id: Optional[int] = None
    automated: Optional[bool] = None


class QuestRead(QuestBase):
    id: int
    created_at: datetime
    updated_at: datetime
    completed_at: Optional[datetime] = None
    template_id: Optional[int] = None
    period_key: Optional[str] = None
    source: Optional[str] = None
    category_slug: Optional[str] = None
    category_label: Optional[str] = None
    category_color: Optional[str] = None
    questline_title: Optional[str] = None
    questline_color: Optional[str] = None
    questline_icon: Optional[str] = None
    questline_icon_url: Optional[str] = None
    steps: List[QuestStepRead] = Field(default_factory=list)
    steps_done: int = 0
    steps_total: int = 0
    progress_label: str = "0 / 0"
    # Derived timer fields (UTC-based; clients format for display).
    remaining_seconds: Optional[int] = None
    timer_tone: Optional[str] = None  # green | orange | red | overdue
    urgent: bool = False

    @field_serializer(
        "created_at",
        "updated_at",
        "completed_at",
        "deadline_at",
        when_used="json",
    )
    def _ser_utc(self, value: Optional[datetime]) -> Optional[str]:
        return to_utc_iso(value)


# --- Periodic templates -------------------------------------------------


class QuestTemplateBase(SQLModel):
    title: str = Field(min_length=1, max_length=200)
    description: str = ""
    pinned: bool = False
    significance: QuestSignificance = QuestSignificance.common
    sort_order: int = 0
    # Local time-of-day for instance deadline ("HH:MM"). Null = no deadline.
    # Ignored when emit_mode=surprise (deadline = appear + duration_seconds).
    deadline_time: Optional[str] = Field(default=None, max_length=8)
    duration_seconds: Optional[int] = Field(default=None, ge=1)
    freq: TemplateFreq = TemplateFreq.daily
    # Comma-separated Mon=0 … Sun=6 (Python weekday). Used when freq=weekly.
    weekdays: str = Field(default="0,1,2,3,4,5,6", max_length=32)
    enabled: bool = True
    timezone: str = Field(default="Europe/Moscow", max_length=64)
    emit_mode: TemplateEmitMode = TemplateEmitMode.fixed
    # Probability of appearing in the period (surprise only). 0..1
    emit_chance: float = Field(default=1.0, ge=0.0, le=1.0)
    # Local window for random scheduled_at ("HH:MM"). Empty → 00:00..23:59.
    emit_window_start: Optional[str] = Field(default=None, max_length=8)
    emit_window_end: Optional[str] = Field(default=None, max_length=8)
    # Shell command run on each roll; stdout must be a JSON array of
    # {title, description?, quest_description?, weight?, ref?}. Independent of
    # emit_mode — a fixed template can pool content too. Empty pool / zero
    # total weight = miss. description → step; quest_description → quest
    # description only when the template description is empty.
    emit_pool_command: Optional[str] = Field(default=None, max_length=20000)
    # How many items to draw from the pool per successful roll.
    # Optional JSON overrides of the emit-output limits (max_steps, max_title, …).
    emit_limits: Optional[str] = Field(default=None, max_length=1000)
    category_id: Optional[int] = Field(
        default=None, foreign_key="questcategory.id", index=True
    )
    questline_id: Optional[int] = Field(
        default=None, foreign_key="questline.id", index=True
    )
    automated: bool = False


class QuestTemplate(QuestTemplateBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    created_at: datetime = Field(default_factory=utcnow)
    updated_at: datetime = Field(default_factory=utcnow)
    steps: List["QuestTemplateStep"] = Relationship(
        back_populates="template",
        sa_relationship_kwargs={
            "cascade": "all, delete-orphan",
            "order_by": "QuestTemplateStep.sort_order",
        },
    )
    category: Optional[QuestCategory] = Relationship()
    questline: Optional[QuestLine] = Relationship()


class QuestTemplateStepBase(SQLModel):
    title: str = Field(min_length=1, max_length=200)
    description: str = ""
    # Inclusive range; materialize picks uniform progress_total in [min, max].
    progress_min: int = Field(default=1, ge=1)
    progress_max: int = Field(default=1, ge=1)
    sort_order: int = 0
    check_command: Optional[str] = Field(default=None, max_length=2000)
    check_interval_seconds: Optional[int] = Field(default=None, ge=15)
    wait_previous: bool = False
    run_mode: str = Field(default="poll", max_length=16)


class QuestTemplateStep(QuestTemplateStepBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    template_id: Optional[int] = Field(
        default=None, foreign_key="questtemplate.id", index=True
    )
    template: Optional[QuestTemplate] = Relationship(back_populates="steps")


class QuestTemplateStepCreate(SQLModel):
    title: str = Field(min_length=1, max_length=200)
    description: str = ""
    progress_min: Optional[int] = Field(default=None, ge=1)
    progress_max: Optional[int] = Field(default=None, ge=1)
    # Shorthand: sets min = max = progress_total when range omitted.
    progress_total: Optional[int] = Field(default=None, ge=1)
    sort_order: int = 0
    check_command: Optional[str] = Field(default=None, max_length=2000)
    check_interval_seconds: Optional[int] = Field(default=None, ge=15)
    wait_previous: bool = False
    run_mode: str = Field(default="poll", max_length=16)


class QuestTemplateStepRead(QuestTemplateStepBase):
    id: int
    template_id: int


class QuestTemplateCreate(QuestTemplateBase):
    steps: List[QuestTemplateStepCreate] = Field(default_factory=list)


class QuestTemplateUpdate(SQLModel):
    title: Optional[str] = Field(default=None, min_length=1, max_length=200)
    description: Optional[str] = None
    pinned: Optional[bool] = None
    significance: Optional[QuestSignificance] = None
    sort_order: Optional[int] = None
    duration_seconds: Optional[int] = Field(default=None, ge=1)
    deadline_time: Optional[str] = Field(default=None, max_length=8)
    freq: Optional[TemplateFreq] = None
    weekdays: Optional[str] = Field(default=None, max_length=32)
    enabled: Optional[bool] = None
    timezone: Optional[str] = Field(default=None, max_length=64)
    emit_mode: Optional[TemplateEmitMode] = None
    emit_chance: Optional[float] = Field(default=None, ge=0.0, le=1.0)
    emit_window_start: Optional[str] = Field(default=None, max_length=8)
    emit_window_end: Optional[str] = Field(default=None, max_length=8)
    emit_pool_command: Optional[str] = Field(default=None, max_length=20000)
    steps: Optional[List[QuestTemplateStepCreate]] = None
    category_id: Optional[int] = None
    questline_id: Optional[int] = None


class QuestTemplateRead(QuestTemplateBase):
    id: int
    created_at: datetime
    updated_at: datetime
    category_slug: Optional[str] = None
    category_label: Optional[str] = None
    category_color: Optional[str] = None
    questline_title: Optional[str] = None
    questline_color: Optional[str] = None
    questline_icon: Optional[str] = None
    questline_icon_url: Optional[str] = None
    steps: List[QuestTemplateStepRead] = Field(default_factory=list)

    @field_serializer("created_at", "updated_at", when_used="json")
    def _ser_utc(self, value: Optional[datetime]) -> Optional[str]:
        return to_utc_iso(value)


class TemplateEmitAttempt(SQLModel, table=True):
    """One emit_pool_command execution (log for explaining a miss/error)."""

    id: Optional[int] = Field(default=None, primary_key=True)
    template_id: int = Field(foreign_key="questtemplate.id")
    period_key: str = Field(max_length=32)
    at: datetime = Field(default_factory=utcnow)
    attempt: int = 1
    # ok | error | timeout | bad_json
    status: str = Field(max_length=16)
    duration_ms: int = 0
    items: int = 0
    picked: int = 0
    # JSON array of picked refs; message is masked (template secrets removed).
    picked_refs: Optional[str] = None
    message: Optional[str] = None


class TemplateEmitRoll(SQLModel, table=True):
    """One surprise roll per template period (miss / wait / done)."""

    __table_args__ = (
        UniqueConstraint(
            "template_id", "period_key", name="uq_template_emit_roll_period"
        ),
    )

    id: Optional[int] = Field(default=None, primary_key=True)
    template_id: int = Field(foreign_key="questtemplate.id", index=True)
    period_key: str = Field(max_length=32, index=True)
    outcome: TemplateEmitOutcome = TemplateEmitOutcome.miss
    # UTC naive; set when outcome=scheduled (and kept after materialize).
    scheduled_at: Optional[datetime] = None
    # emit_pool_command failures this period (capped at 3, then outcome=error).
    attempts: int = 0
    # Earliest time of the next retry after a failed run (UTC naive); NULL when
    # nothing is pending.
    retry_at: Optional[datetime] = None
    # JSON array of chosen items' `ref` (or a title/description hash when no
    # ref was given), used to exclude recent picks on later periods.
    picked_refs: Optional[str] = None
    created_at: datetime = Field(default_factory=utcnow)
    updated_at: datetime = Field(default_factory=utcnow)


# --- Quest change log (append-only activity) ------------------------------


class QuestChangeLog(SQLModel, table=True):
    """Durable log of quest lifecycle / edits (not intra-step progress)."""

    id: Optional[int] = Field(default=None, primary_key=True)
    at: datetime = Field(default_factory=utcnow, index=True)
    kind: str = Field(max_length=32, index=True)
    quest_id: Optional[int] = Field(
        default=None,
        foreign_key="quest.id",
        index=True,
    )
    title: str = Field(default="", max_length=200)
    detail: str = Field(default="", max_length=500)
    significance: Optional[str] = Field(default=None, max_length=16)
    # Monotonic hub revision when published live; null if backfilled later.
    revision: Optional[int] = Field(default=None, index=True)
    # What the entry is about: quest | note | attachment. quest_id stays for quests.
    entity_type: str = Field(default="quest", max_length=16)
    entity_id: Optional[int] = Field(default=None)
    # Owner of an attachment (quest | questline | note), for history in its card.
    parent_type: Optional[str] = Field(default=None, max_length=16)
    parent_id: Optional[int] = Field(default=None)


class QuestChangeLogRead(SQLModel):
    id: int
    at: datetime
    kind: str
    quest_id: Optional[int] = None
    title: str
    detail: str
    significance: Optional[str] = None
    revision: Optional[int] = None
    entity_type: str = "quest"
    entity_id: Optional[int] = None
    parent_type: Optional[str] = None
    parent_id: Optional[int] = None

    @field_serializer("at", when_used="json")
    def _ser_at(self, value: Optional[datetime]) -> Optional[str]:
        return to_utc_iso(value)


class AppUser(SQLModel, table=True):
    """Account for the web UI. Data itself is shared — this gates access, not ownership."""

    id: Optional[int] = Field(default=None, primary_key=True)
    username: str = Field(max_length=64, unique=True, index=True)
    # pbkdf2_sha256$<iters>$<salt_b64>$<hash_b64>, written/verified by the Go server.
    password_hash: str = Field(max_length=256)
    is_active: bool = Field(default=True)
    created_at: datetime = Field(default_factory=utcnow)
    last_login_at: Optional[datetime] = Field(default=None)


class UserSession(SQLModel, table=True):
    """Browser session behind the cookie. Stored so sessions can be revoked server-side."""

    id: Optional[int] = Field(default=None, primary_key=True)
    user_id: int = Field(foreign_key="appuser.id", index=True)
    token_hash: str = Field(max_length=64, unique=True, index=True)
    created_at: datetime = Field(default_factory=utcnow)
    expires_at: datetime
    last_seen_at: Optional[datetime] = Field(default=None)
    user_agent: str = Field(default="", max_length=256)


class ApiToken(SQLModel, table=True):
    """Long-lived bearer token for headless clients (CLI, overlay, bot, MCP)."""

    id: Optional[int] = Field(default=None, primary_key=True)
    user_id: int = Field(foreign_key="appuser.id", index=True)
    name: str = Field(max_length=64)
    token_hash: str = Field(max_length=64, unique=True, index=True)
    created_at: datetime = Field(default_factory=utcnow)
    last_used_at: Optional[datetime] = Field(default=None)
    revoked_at: Optional[datetime] = Field(default=None)


# --- Attachments ----------------------------------------------------------


class AttachmentOwner(str, Enum):
    quest = "quest"
    questline = "questline"
    note = "note"


class AttachmentScanStatus(str, Enum):
    """ClamAV verdict. A row only ever reaches the DB as clean — the other
    states exist for rows whose scan could not be completed."""

    pending = "pending"
    clean = "clean"
    infected = "infected"
    error = "error"


class Attachment(SQLModel, table=True):
    """Logical file card on a quest / questline / note.

    The *current* revision's bytes live on WebDAV; older revisions are in
    ``attachment_revision``. Fields below mirror the current revision so
    listings stay a single-row read. ``attachment=N`` refs point at this id.
    """

    id: Optional[int] = Field(default=None, primary_key=True)
    owner_type: str = Field(max_length=16, index=True)
    owner_id: int = Field(index=True)
    # Original name, shown to humans. Never used to build the storage path.
    filename: str = Field(max_length=255)
    # Path inside the WebDAV root for the *current* revision.
    webdav_path: str = Field(max_length=512, unique=True, index=True)
    size_bytes: int = Field(default=0)
    content_type_declared: str = Field(default="", max_length=128)
    content_type_detected: str = Field(default="", max_length=128)
    comment: str = Field(default="", max_length=500)
    uploaded_at: datetime = Field(default_factory=utcnow)
    scan_status: str = Field(default="pending", max_length=16)
    scanned_at: Optional[datetime] = Field(default=None)
    # 1-based index of the revision currently mirrored on this row.
    current_revision: int = Field(default=1)
    # Hidden cards show only on the Attachments tab (soft alternative to delete).
    hidden: bool = Field(default=False)


class AttachmentRevision(SQLModel, table=True):
    """One immutable version of an attachment's bytes on WebDAV."""

    __tablename__ = "attachment_revision"
    __table_args__ = (
        UniqueConstraint("attachment_id", "revision", name="uq_attachment_revision"),
    )

    id: Optional[int] = Field(default=None, primary_key=True)
    attachment_id: int = Field(foreign_key="attachment.id", index=True)
    revision: int = Field(default=1)
    filename: str = Field(max_length=255)
    webdav_path: str = Field(max_length=512, unique=True)
    size_bytes: int = Field(default=0)
    content_type_declared: str = Field(default="", max_length=128)
    content_type_detected: str = Field(default="", max_length=128)
    comment: str = Field(default="", max_length=500)
    uploaded_at: datetime = Field(default_factory=utcnow)
    scan_status: str = Field(default="pending", max_length=16)
    scanned_at: Optional[datetime] = Field(default=None)


class AttachmentRead(SQLModel):
    id: int
    owner_type: str
    owner_id: int
    filename: str
    size_bytes: int
    content_type_declared: str
    content_type_detected: str
    comment: str
    uploaded_at: datetime
    scan_status: str
    scanned_at: Optional[datetime] = None
    current_revision: int = 1
    revision_count: int = 1
    hidden: bool = False

    @field_serializer("uploaded_at", "scanned_at", when_used="json")
    def _ser_utc(self, value: Optional[datetime]) -> Optional[str]:
        return to_utc_iso(value)
