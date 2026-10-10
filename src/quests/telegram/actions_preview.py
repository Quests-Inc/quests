"""Format LLM action-batch preview for Telegram HTML."""

from __future__ import annotations

import os
from datetime import UTC, datetime
from html import escape
from typing import Any
from zoneinfo import ZoneInfo

SIGNIFICANCE_LABEL_RU: dict[str, str] = {
    "insignificant": "незначительное",
    "common": "обычное",
    "uncommon": "необычное",
    "epic": "эпическое",
    "legendary": "легендарное",
}

_ACTION_RU = {
    "create_questline": "создать квестлайн",
    "create_quest": "создать квест",
    "add_step": "добавить шаг",
    "update_step": "изменить шаг",
    "delete_step": "удалить шаг",
    "update_quest": "изменить квест",
}


def _esc(s: Any) -> str:
    return escape(str(s or ""), quote=False)


_FIELD_RU = {
    "title": "название",
    "description": "описание",
    "status": "статус",
    "significance": "важность",
    "pinned": "закреплён",
    "deadline_at": "дедлайн",
    "duration_seconds": "окно",
    "category": "раздел",
    "category_id": "раздел",
    "questline": "квестлайн",
    "questline_id": "квестлайн",
    "sort_order": "порядок",
    "progress_current": "прогресс",
    "progress_total": "всего",
}
# Fields shown for a change, in this order; the rest of `after` is bookkeeping.


def _local(iso: Any) -> str:
    """UTC ISO → `дд.мм чч:мм` in the bot's timezone; unparseable stays as is."""
    try:
        dt = datetime.fromisoformat(str(iso).replace("Z", "+00:00"))
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=UTC)
        try:
            tz = ZoneInfo(os.environ.get("QUESTS_TZ") or "Europe/Moscow")
        except (KeyError, ValueError):
            tz = UTC
        return dt.astimezone(tz).strftime("%d.%m %H:%M")
    except ValueError:
        return str(iso)


_SHOWN = tuple(_FIELD_RU)
_TG_LIMIT = 3900


def _clip(s: Any, n: int) -> str:
    t = " ".join(str("" if s is None else s).split())
    return t if len(t) <= n else t[: n - 1] + "…"


def _val(key: str, v: Any, desc_len: int) -> str:
    if v is None or v == "":
        return "—"
    if key == "significance":
        return _esc(SIGNIFICANCE_LABEL_RU.get(str(v), str(v)))
    if key == "pinned":
        return "да" if v else "нет"
    if key == "deadline_at":
        return _esc(_local(v))
    if key == "duration_seconds":
        m = int(v) // 60
        return f"{m // 60} ч {m % 60} мин" if m >= 60 else f"{m} мин"
    if key == "description":
        return _esc(_clip(v, desc_len))
    return _esc(_clip(v, 120))


def _fields(act: dict[str, Any], desc_len: int) -> list[str]:
    """`ключ: значение` rows for everything the action sets, minus the title."""
    rows: list[str] = []
    seen: set[str] = set()
    for k in _SHOWN:
        if k in ("title", "progress_current", "progress_total") or k in seen:
            continue
        v = act.get(k)
        if k == "pinned" and v is None:
            continue
        if v is None or v == "":
            continue
        if k in ("category", "category_id"):
            seen.update(("category", "category_id"))
        if k in ("questline", "questline_id"):
            seen.update(("questline", "questline_id"))
        label = _FIELD_RU[k]
        if k == "category_id":
            v = f"id={v}"
        if k == "questline_id":
            v = f"id={v}"
        rows.append(f"   {label}: {_val(k, v, desc_len)}")
    if act.get("clear_questline"):
        rows.append("   квестлайн: убрать")
    return rows


def _step_line(st: dict[str, Any], desc_len: int) -> list[str]:
    total = int(st.get("progress_total") or 1)
    cur = int(st.get("progress_current") or 0)
    prog = f" ({cur}/{total})" if total > 1 or cur else ""
    out = [f"   · {_esc(st.get('title') or 'шаг')}{prog}"]
    if st.get("description"):
        out.append(f"      <i>{_esc(_clip(st['description'], desc_len))}</i>")
    return out


def _diff(act: dict[str, Any], old: dict[str, Any] | None, desc_len: int) -> list[str]:
    """Changed fields as `было → стало` against the current state."""
    rows: list[str] = []
    old = old or {}
    for k in _SHOWN:
        if k not in act or act[k] is None:
            continue
        if k in ("category", "questline"):
            continue  # names are resolved server-side; ids below show the change
        new = act[k]
        before = old.get(k)
        if before == new:
            continue
        rows.append(
            f"   {_FIELD_RU[k]}: {_val(k, before, desc_len)} → <b>{_val(k, new, desc_len)}</b>"
        )
    if act.get("category") and not act.get("category_id"):
        rows.append(f"   раздел → <b>{_esc(act['category'])}</b>")
    if act.get("questline") and not act.get("questline_id"):
        rows.append(f"   квестлайн → <b>{_esc(act['questline'])}</b>")
    if act.get("clear_questline"):
        rows.append("   квестлайн → убрать")
    return rows


def _build(batch: dict[str, Any], preview: list[dict[str, Any]], desc_len: int, max_steps: int) -> list[str]:
    actions = list((batch or {}).get("actions") or [])
    by_index = {int(r.get("index", -1)): r for r in preview}
    lines = ["<b>План действий</b>"]
    for act in actions:
        idx = int(act.get("index", 0))
        kind = str(act.get("action") or "?")
        pr = by_index.get(idx) or {}
        before = pr.get("before") or {}
        bits = [f"<b>{idx + 1}.</b> {_ACTION_RU.get(kind, kind)}"]
        if act.get("title") and kind in ("create_quest", "create_questline"):
            bits.append(f"«{_esc(act['title'])}»")
        if act.get("quest_id") is not None:
            name = before.get("title") if kind != "create_quest" else None
            bits.append(f"quest={act['quest_id']}" + (f" «{_esc(name)}»" if name else ""))
        if act.get("questline_id") is not None and kind not in ("create_quest", "update_quest"):
            bits.append(f"questline={act['questline_id']}")
        if act.get("step_id") is not None:
            bits.append(f"step={act['step_id']}")
        if act.get("questline_id_ref") is not None:
            bits.append(f"→квестлайн[{act['questline_id_ref']}]")
        if act.get("quest_id_ref") is not None:
            bits.append(f"→квест[{act['quest_id_ref']}]")
        lines.append(" ".join(bits))

        old_step = None
        if act.get("step_id") is not None:
            for st in before.get("steps") or []:
                if st.get("id") == act["step_id"]:
                    old_step = st
                    break

        if kind in ("create_quest", "create_questline"):
            lines.extend(_fields(act, desc_len))
        elif kind == "update_quest":
            lines.extend(_diff(act, before, desc_len))
        elif kind == "update_step":
            if old_step and old_step.get("title"):
                lines.append(f"   шаг: «{_esc(_clip(old_step['title'], 80))}»")
            lines.extend(_diff(act, old_step, desc_len))
        elif kind == "add_step":
            lines.extend(_step_line(act, desc_len))
        elif kind == "delete_step":
            if old_step:
                lines.extend(_step_line(old_step, desc_len)[:1])

        steps = act.get("steps") or []
        for st in steps[:max_steps]:
            lines.extend(_step_line(st, desc_len))
        if len(steps) > max_steps:
            lines.append(f"   · … ещё {len(steps) - max_steps}")
    lines.append("")
    lines.append("Применить этот план?")
    return lines


def format_actions_preview(
    batch: dict[str, Any],
    preview: list[dict[str, Any]] | None = None,
) -> str:
    """Human-readable plan from ActionBatch (+ dry-run rows with before/after).

    Shows everything each action sets (description, importance, deadline, step
    details) and, for edits, what changes as `было → стало`. Telegram caps a
    message at 4096 chars: descriptions shrink first, then the tail is cut on a
    line boundary (every line is tag-balanced).
    """
    if not list((batch or {}).get("actions") or []):
        return "<b>План действий</b>\n<i>(пусто)</i>"
    rows = list(preview or [])
    for desc_len, max_steps in ((300, 12), (120, 8), (60, 5)):
        lines = _build(batch, rows, desc_len, max_steps)
        text = "\n".join(lines)
        if len(text) <= _TG_LIMIT:
            return text
    keep, size = [], 0
    for ln in lines[:-2]:
        if size + len(ln) + 1 > _TG_LIMIT - 120:
            break
        keep.append(ln)
        size += len(ln) + 1
    keep.append("<i>… план обрезан по размеру сообщения</i>")
    keep.extend(["", "Применить этот план?"])
    return "\n".join(keep)


def history_to_prompt_text(
    user_text: str,
    history: list[tuple[str, str]] | None,
) -> str:
    """Fold clarify turns into one user message (Go preview has no history yet)."""
    if not history:
        return user_text.strip()
    parts: list[str] = ["Предыдущий диалог:"]
    for role, content in history:
        r = "пользователь" if role == "user" else "ассистент"
        parts.append(f"{r}: {content}")
    parts.append("")
    parts.append("Текущий запрос:")
    parts.append(user_text.strip())
    return "\n".join(parts)
