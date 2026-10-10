"""Drag / nudge helpers for layer-shell HUD — within the current monitor only."""

from __future__ import annotations

from collections.abc import Callable
from typing import Any

from gi.repository import Gtk
from gi.repository import Gtk4LayerShell as LayerShell

from .monitors import list_monitors

# Keyboard nudge step (layout pixels).
NUDGE_STEP = 24


def _window_size(window: Gtk.Window) -> tuple[int, int]:
    surface = window.get_surface()
    w = h = 0
    if surface is not None:
        w = int(surface.get_width() or 0)
        h = int(surface.get_height() or 0)
    if w <= 0:
        w = int(window.get_width() or 0)
    if h <= 0:
        h = int(window.get_height() or 0)
    if w <= 0 or h <= 0:
        alloc = window.get_allocation()
        w = max(w, int(alloc.width or 0))
        h = max(h, int(alloc.height or 0))
    return max(1, w), max(1, h)


def _geo(mon) -> tuple[int, int, int, int]:
    g = mon.get_geometry()
    return int(g.x), int(g.y), int(g.width), int(g.height)


def _set_margins(window: Gtk.Window, margin_top: int, margin_right: int) -> None:
    LayerShell.set_margin(window, LayerShell.Edge.TOP, margin_top)
    LayerShell.set_margin(window, LayerShell.Edge.RIGHT, margin_right)
    LayerShell.set_margin(window, LayerShell.Edge.BOTTOM, 0)
    LayerShell.set_margin(window, LayerShell.Edge.LEFT, 0)


def nudge_hud(
    window: Gtk.Window,
    display,
    state: dict[str, Any],
    *,
    dx: int = 0,
    dy: int = 0,
    on_moved: Callable[[], None] | None = None,
) -> bool:
    """Move HUD by (dx, dy) screen pixels; clamped to the active monitor.

    Positive dx → right, positive dy → down. Returns True if margins changed.
    """
    if dx == 0 and dy == 0:
        return False
    mons = list_monitors(display)
    if not mons:
        return False
    idx = int(state.get("monitor_index", 0)) % len(mons)
    mon = mons[idx]
    win_w, win_h = _window_size(window)
    _gx, _gy, gw, gh = _geo(mon)
    max_mt = max(0, gh - win_h)
    max_mr = max(0, gw - win_w)
    mt = LayerShell.get_margin(window, LayerShell.Edge.TOP)
    mr = LayerShell.get_margin(window, LayerShell.Edge.RIGHT)
    # TOP+RIGHT anchors: right → smaller margin_right; down → larger margin_top.
    new_mt = min(max(0, mt + dy), max_mt)
    new_mr = min(max(0, mr - dx), max_mr)
    if new_mt == mt and new_mr == mr:
        return False
    _set_margins(window, new_mt, new_mr)
    state["margin_top"] = new_mt
    state["margin_right"] = new_mr
    if on_moved is not None:
        on_moved()
    return True


def attach_drag_handle(
    handle: Gtk.Widget,
    *,
    window: Gtk.Window,
    display,
    state: dict[str, Any],
    on_moved: Callable[[], None] | None = None,
) -> None:
    """Wire GestureDrag: TOP/RIGHT margins only, hard-clamped to the active monitor."""
    gesture = Gtk.GestureDrag.new()
    gesture.set_button(1)
    handle.add_controller(gesture)

    drag: dict[str, Any] = {}

    def _notify() -> None:
        if on_moved is not None:
            on_moved()

    def on_begin(_g, *_args) -> None:
        mons = list_monitors(display)
        if not mons:
            return
        idx = int(state.get("monitor_index", 0)) % len(mons)
        mon = mons[idx]
        win_w, win_h = _window_size(window)
        _gx, _gy, gw, gh = _geo(mon)
        max_mt = max(0, gh - win_h)
        max_mr = max(0, gw - win_w)
        mt = min(max(0, LayerShell.get_margin(window, LayerShell.Edge.TOP)), max_mt)
        mr = min(max(0, LayerShell.get_margin(window, LayerShell.Edge.RIGHT)), max_mr)
        _set_margins(window, mt, mr)
        gx, gy, gw, _gh = _geo(mon)
        state["dragging"] = True
        drag.clear()
        drag.update(
            {
                "abs_x": gx + gw - win_w - mr,
                "abs_y": gy + mt,
                "win_w": win_w,
                "win_h": win_h,
                "max_mt": max_mt,
                "max_mr": max_mr,
                "gx": gx,
                "gy": gy,
                "gw": gw,
            }
        )

    def on_update(_g, offset_x: float, offset_y: float) -> None:
        if "abs_x" not in drag:
            return
        win_w = int(drag["win_w"])
        max_mt = int(drag["max_mt"])
        max_mr = int(drag["max_mr"])
        gx = int(drag["gx"])
        gy = int(drag["gy"])
        gw = int(drag["gw"])

        new_x = int(round(float(drag["abs_x"]) + offset_x))
        new_y = int(round(float(drag["abs_y"]) + offset_y))
        mt_raw = new_y - gy
        mr_raw = (gx + gw) - (new_x + win_w)
        mt = min(max(0, mt_raw), max_mt)
        mr = min(max(0, mr_raw), max_mr)

        _set_margins(window, mt, mr)

        # At the edge: re-base so further outward drag does not accumulate slack
        # and cannot yank the surface off-screen when the pointer jumps.
        if mt != mt_raw or mr != mr_raw:
            drag["abs_x"] = (gx + gw - win_w - mr) - offset_x
            drag["abs_y"] = (gy + mt) - offset_y

        _notify()

    def on_end(_g, *_args) -> None:
        state["dragging"] = False
        drag.clear()
        _notify()

    gesture.connect("drag-begin", on_begin)
    gesture.connect("drag-update", on_update)
    gesture.connect("drag-end", on_end)
