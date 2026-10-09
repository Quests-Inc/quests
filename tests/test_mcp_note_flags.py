import pytest

from quests import mcp_server as m


ROWS = [
    {"id": 1, "outdated": True, "is_private": False, "pinned": False},
    {"id": 2, "outdated": False, "is_private": True, "pinned": True},
    {"id": 3, "outdated": False, "is_private": False, "pinned": False},
]


def ids(flags):
    return [n["id"] for n in m._filter_note_flags(ROWS, flags)]


def test_flag_filter():
    assert ids("") == [1, 2, 3]
    assert ids("outdated") == [1]
    assert ids("stale") == [1]
    assert ids("-outdated,-private") == [3]
    assert ids("pinned,private") == [2]


def test_unknown_flag_is_an_error():
    with pytest.raises(ValueError):
        ids("nope")
