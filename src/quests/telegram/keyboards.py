"""Keyboards: main reply + quest inline (status / steps / paging)."""

from __future__ import annotations

from aiogram.types import (
    CopyTextButton,
    InlineKeyboardButton,
    InlineKeyboardMarkup,
    KeyboardButton,
    ReplyKeyboardMarkup,
)

# Reply (общие функции)
BTN_LIST = "📋 Список"
BTN_NEW = "➕ Новая"
BTN_NEW_LLM = "✨ LLM"
BTN_HELP = "❓ Помощь"
BTN_CANCEL = "✖ Отмена"
BTN_EDIT = "✏️ Редактировать"

# Inline: statuses — qs:<qid>:<status>
STATUSES = (
    ("completed", "✓ Выполнено"),
    ("failed", "✗ Провал"),
    ("expired", "⏳ Просрочено"),
    ("frozen", "🧊 Заморожено"),
    ("active", "▶ Активно"),
)

# Шагов на странице (статусы + refresh + nav уже занимают ряды;
# лимит Telegram ~100 кнопок, UX важнее).
STEPS_PER_PAGE = 5


def main_reply_keyboard() -> ReplyKeyboardMarkup:
    return ReplyKeyboardMarkup(
        keyboard=[
            [KeyboardButton(text=BTN_LIST), KeyboardButton(text=BTN_NEW)],
            [KeyboardButton(text=BTN_NEW_LLM), KeyboardButton(text=BTN_HELP)],
            [KeyboardButton(text=BTN_CANCEL)],
        ],
        resize_keyboard=True,
        is_persistent=True,
        input_field_placeholder="задача текстом → LLM",
    )


def llm_confirm_keyboard(
    *,
    index: int = 0,
    total: int = 1,
) -> InlineKeyboardMarkup:
    """Two rows: Create/Cancel, then Prev/Next (edges omitted)."""
    row1 = [
        InlineKeyboardButton(text="✓ Применить", callback_data="llm:ok"),
        InlineKeyboardButton(text="✖ Отмена", callback_data="llm:no"),
    ]
    rows: list[list[InlineKeyboardButton]] = [row1]
    if total > 1:
        row2: list[InlineKeyboardButton] = []
        if index > 0:
            row2.append(
                InlineKeyboardButton(text="← Предыдущий", callback_data="llm:prev")
            )
        if index < total - 1:
            row2.append(
                InlineKeyboardButton(text="Следующий →", callback_data="llm:next")
            )
        if row2:
            rows.append(row2)
    return InlineKeyboardMarkup(inline_keyboard=rows)


# Статусы, у которых по умолчанию только «Редактировать» (без шагов/статусов).
_CLOSED_STATUSES = frozenset({"completed", "failed", "archived"})


def _is_closed(quest: dict) -> bool:
    return str(quest.get("status") or "") in _CLOSED_STATUSES


def _step_done(step: dict) -> bool:
    if step.get("done") is True:
        return True
    try:
        return int(step.get("progress_current") or 0) >= int(
            step.get("progress_total") or 1
        )
    except (TypeError, ValueError):
        return False


def _clamp_page(page: int, n_steps: int) -> int:
    if n_steps <= 0:
        return 0
    pages = max(1, (n_steps + STEPS_PER_PAGE - 1) // STEPS_PER_PAGE)
    return max(0, min(int(page), pages - 1))


def _copy_btn(label: str, payload: str) -> InlineKeyboardButton:
    """Telegram CopyTextButton — taps copy entity=N into the clipboard."""
    text = payload[:256]
    shown = label if len(label) <= 28 else label[:25] + "…"
    return InlineKeyboardButton(text=shown, copy_text=CopyTextButton(text=text))


def entity_copy_rows(quest: dict) -> list[list[InlineKeyboardButton]]:
    """Rows of copy-buttons for quest / questline / steps (for LLM «Команда»)."""
    rows: list[list[InlineKeyboardButton]] = []
    qid = quest.get("id")
    if qid is not None:
        rows.append([_copy_btn(f"quest={qid}", f"quest={int(qid)}")])
    lid = quest.get("questline_id")
    if lid is not None:
        rows.append([_copy_btn(f"questline={int(lid)}", f"questline={int(lid)}")])
    step_btns: list[InlineKeyboardButton] = []
    for step in list(quest.get("steps") or [])[:8]:
        sid = step.get("id")
        if sid is None:
            continue
        step_btns.append(_copy_btn(f"step={int(sid)}", f"step={int(sid)}"))
        if len(step_btns) == 3:
            rows.append(step_btns)
            step_btns = []
    if step_btns:
        rows.append(step_btns)
    return rows


def quest_keyboard(
    quest: dict, *, page: int = 0, expanded: bool = False
) -> InlineKeyboardMarkup:
    """Статусы + бинарные шаги. Закрытые по умолчанию — только «Редактировать»."""
    qid = int(quest["id"])
    copy_rows = entity_copy_rows(quest)
    if _is_closed(quest) and not expanded:
        return InlineKeyboardMarkup(
            inline_keyboard=[
                *copy_rows,
                [
                    InlineKeyboardButton(
                        text=BTN_EDIT,
                        callback_data=f"qe:{qid}",
                    )
                ],
            ]
        )

    steps = list(quest.get("steps") or [])
    page = _clamp_page(page, len(steps))

    rows: list[list[InlineKeyboardButton]] = [*copy_rows]
    for i in range(0, len(STATUSES), 2):
        pair = STATUSES[i : i + 2]
        rows.append(
            [
                InlineKeyboardButton(
                    text=label,
                    callback_data=f"qs:{qid}:{status}",
                )
                for status, label in pair
            ]
        )

    if steps:
        start = page * STEPS_PER_PAGE
        chunk = steps[start : start + STEPS_PER_PAGE]
        for step in chunk:
            sid = int(step["id"])
            title = str(step.get("title") or f"#{sid}")
            if len(title) > 36:
                title = title[:33] + "…"
            mark = "✅" if _step_done(step) else "⬜"
            rows.append(
                [
                    InlineKeyboardButton(
                        text=f"{mark} {title}",
                        callback_data=f"qt:{qid}:{sid}:{page}",
                    )
                ]
            )

        pages = max(1, (len(steps) + STEPS_PER_PAGE - 1) // STEPS_PER_PAGE)
        if pages > 1:
            nav: list[InlineKeyboardButton] = []
            if page > 0:
                nav.append(
                    InlineKeyboardButton(
                        text="‹ Назад",
                        callback_data=f"qp:{qid}:{page - 1}",
                    )
                )
            nav.append(
                InlineKeyboardButton(
                    text=f"{page + 1}/{pages}",
                    callback_data=f"qp:{qid}:{page}",
                )
            )
            if page < pages - 1:
                nav.append(
                    InlineKeyboardButton(
                        text="Вперёд ›",
                        callback_data=f"qp:{qid}:{page + 1}",
                    )
                )
            rows.append(nav)

    rows.append(
        [
            InlineKeyboardButton(
                text="🔄 Обновить",
                callback_data=f"qr:{qid}:{page}",
            ),
        ]
    )
    return InlineKeyboardMarkup(inline_keyboard=rows)


def quest_pick_keyboard(quests: list[dict], *, limit: int = 20) -> InlineKeyboardMarkup:
    """One button per quest to open card + status controls."""
    rows: list[list[InlineKeyboardButton]] = []
    for q in quests[:limit]:
        qid = int(q["id"])
        title = str(q.get("title") or "?")
        if len(title) > 40:
            title = title[:37] + "…"
        rows.append(
            [
                InlineKeyboardButton(
                    text=f"#{qid} {title}",
                    callback_data=f"qo:{qid}",
                )
            ]
        )
    return InlineKeyboardMarkup(inline_keyboard=rows)


def category_pick_keyboard(categories: list[dict]) -> InlineKeyboardMarkup:
    rows: list[list[InlineKeyboardButton]] = []
    for c in categories:
        cid = int(c["id"])
        label = str(c.get("label") or c.get("slug") or cid)
        rows.append(
            [
                InlineKeyboardButton(
                    text=label,
                    callback_data=f"cc:{cid}",
                )
            ]
        )
    rows.append(
        [InlineKeyboardButton(text="Без раздела", callback_data="cc:0")]
    )
    rows.append(
        [InlineKeyboardButton(text="✖ Отмена", callback_data="cc:cancel")]
    )
    return InlineKeyboardMarkup(inline_keyboard=rows)
