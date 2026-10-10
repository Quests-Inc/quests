"""Deadline timer helpers for the overlay (API sends UTC ISO)."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta


def _parse_utc(value) -> datetime | None:
    if value is None:
        return None
    if isinstance(value, datetime):
        dt = value
    else:
        text = str(value).strip()
        if text.endswith("Z"):
            text = text[:-1] + "+00:00"
        try:
            dt = datetime.fromisoformat(text)
        except ValueError:
            return None
    if dt.tzinfo is None:
        return dt.replace(tzinfo=UTC)
    return dt.astimezone(UTC)


def remaining_seconds(deadline_at, *, now: datetime | None = None) -> int | None:
    deadline = _parse_utc(deadline_at)
    if deadline is None:
        return None
    now = now or datetime.now(UTC)
    return int((deadline - now).total_seconds())


def is_urgent(deadline_at, duration_seconds, *, now: datetime | None = None) -> bool:
    """True in the active window [deadline−duration, deadline] or after deadline."""
    rem = remaining_seconds(deadline_at, now=now)
    if rem is not None and rem <= 0:
        return True
    deadline = _parse_utc(deadline_at)
    if deadline is None or not duration_seconds:
        return False
    now = now or datetime.now(UTC)
    start = deadline - timedelta(seconds=max(1, int(duration_seconds)))
    return start < now


def timer_tone(deadline_at, duration_seconds, *, now: datetime | None = None) -> str | None:
    rem = remaining_seconds(deadline_at, now=now)
    if rem is None or not duration_seconds:
        return None
    frac = rem / max(1, int(duration_seconds))
    if frac > 2 / 3:
        return "green"
    if frac > 1 / 3:
        return "orange"
    return "red"


def format_remaining(seconds: int | None) -> str:
    if seconds is None:
        return ""
    sign = "-" if seconds < 0 else ""
    s = abs(int(seconds))
    h, rem = divmod(s, 3600)
    m, sec = divmod(rem, 60)
    if h:
        return f"{sign}{h}:{m:02d}:{sec:02d}"
    return f"{sign}{m}:{sec:02d}"
