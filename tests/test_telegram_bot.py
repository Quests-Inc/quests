"""Telegram notify routing + completed-quest keyboards (no live API)."""

from __future__ import annotations

from quests.telegram.keyboards import BTN_EDIT, quest_keyboard
from quests.telegram.notify import should_push_event


def test_skip_quiet_daily_appeared() -> None:
    assert not should_push_event(
        {"kind": "quest_appeared", "toast": False, "title": "Зубы"}
    )


def test_skip_automated_appear_start() -> None:
    assert not should_push_event(
        {"kind": "quest_appeared", "toast": True, "automated": True}
    )
    assert not should_push_event(
        {"kind": "quest_started", "toast": True, "automated": True}
    )


def test_push_human_complete_and_start() -> None:
    assert should_push_event({"kind": "quest_completed", "toast": True})
    assert should_push_event(
        {"kind": "quest_completed", "toast": True, "automated": True}
    )
    assert should_push_event({"kind": "quest_started", "toast": True})
    assert should_push_event({"kind": "quest_appeared", "toast": True})


def test_completed_keyboard_collapsed() -> None:
    q = {"id": 7, "status": "completed", "steps": [{"id": 1, "title": "a"}]}
    kb = quest_keyboard(q)
    rows = kb.inline_keyboard
    # copy quest= / step= rows, then Edit
    assert len(rows) >= 2
    assert rows[0][0].text == "quest=7"
    assert rows[0][0].copy_text is not None
    assert rows[0][0].copy_text.text == "quest=7"
    assert rows[-1][0].text == BTN_EDIT
    assert rows[-1][0].callback_data == "qe:7"


def test_failed_keyboard_collapsed() -> None:
    q = {"id": 8, "status": "failed", "steps": [{"id": 1, "title": "a"}]}
    kb = quest_keyboard(q)
    assert kb.inline_keyboard[0][0].text == "quest=8"
    assert kb.inline_keyboard[-1][0].text == BTN_EDIT
    assert kb.inline_keyboard[-1][0].callback_data == "qe:8"


def test_completed_keyboard_expanded_has_done() -> None:
    q = {"id": 7, "status": "completed", "steps": []}
    kb = quest_keyboard(q, expanded=True)
    labels = [btn.text for row in kb.inline_keyboard for btn in row]
    assert "✓ Выполнено" in labels
    assert BTN_EDIT not in labels


def test_active_keyboard_always_full() -> None:
    q = {"id": 3, "status": "active", "steps": []}
    kb = quest_keyboard(q)
    labels = [btn.text for row in kb.inline_keyboard for btn in row]
    assert "✓ Выполнено" in labels
    assert "▶ Активно" in labels
