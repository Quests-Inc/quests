"""Quests marked overlay_hidden (crossed-out pin) never reach any HUD lane."""

import pytest

pytest.importorskip("gi")  # the overlay needs system GTK; absent in the uv venv

from overlay.hud.panel import split_hud_quests  # noqa: E402


def _quest(qid, **kw):
    q = {
        "id": qid,
        "title": f"q{qid}",
        "status": "active",
        "steps": [{"id": qid * 10, "title": "s", "done": False, "progress_current": 0, "progress_total": 1}],
    }
    q.update(kw)
    return q


def test_hidden_quest_is_in_no_lane():
    items = [
        _quest(1, pinned=True),
        _quest(2, overlay_hidden=True),
        _quest(3, status="expired", overlay_hidden=True),
        _quest(4, status="expired"),
    ]
    favorites, urgent, category = split_hud_quests(items)
    ids = {e.quest_id for e in favorites + urgent + category}
    assert 1 in ids and 4 in ids
    assert 2 not in ids and 3 not in ids
