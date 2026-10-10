"""Multi-monitor helpers for layer-shell surfaces."""

from __future__ import annotations

import shutil
import subprocess

from gi.repository import Gdk, Gtk
from gi.repository import Gtk4LayerShell as LayerShell


def list_monitors(display: Gdk.Display | None = None) -> list[Gdk.Monitor]:
    display = display or Gdk.Display.get_default()
    if display is None:
        return []
    model = display.get_monitors()
    return [model.get_item(i) for i in range(model.get_n_items())]


def monitor_label(monitor: Gdk.Monitor | None, index: int, total: int) -> str:
    if monitor is None:
        return f"M{index + 1}/{total}" if total else "—"
    connector = ""
    if hasattr(monitor, "get_connector"):
        connector = (monitor.get_connector() or "").strip()
    if connector:
        # Prefer short connector (DP-2, HDMI-A-1).
        return connector
    return f"M{index + 1}/{total}" if total else f"M{index + 1}"


def apply_monitor(window: Gtk.Window, monitor: Gdk.Monitor | None) -> None:
    LayerShell.set_monitor(window, monitor)


def monitor_connector(monitor: Gdk.Monitor | None) -> str:
    if monitor is None:
        return ""
    if hasattr(monitor, "get_connector"):
        return (monitor.get_connector() or "").strip()
    return ""


def resolve_monitor_index(
    mons: list[Gdk.Monitor],
    *,
    connector: str = "",
    index: int = 0,
) -> int:
    """Prefer connector match; else clamp index."""
    if not mons:
        return 0
    want = (connector or "").strip()
    if want:
        for i, mon in enumerate(mons):
            if monitor_connector(mon) == want:
                return i
    return int(index) % len(mons)


def cycle_index(current: int, total: int) -> int:
    if total <= 0:
        return 0
    return (current + 1) % total


def monitor_at_point(
    display: Gdk.Display | None, x: int, y: int
) -> tuple[int, Gdk.Monitor] | None:
    """Return (index, monitor) whose geometry contains layout point (x, y)."""
    mons = list_monitors(display)
    for i, mon in enumerate(mons):
        g = mon.get_geometry()
        if int(g.x) <= x < int(g.x) + int(g.width) and int(g.y) <= y < int(g.y) + int(
            g.height
        ):
            return i, mon
    return None


def neighbor_monitor(
    mons: list[Gdk.Monitor], idx: int, direction: str
) -> tuple[int, Gdk.Monitor] | None:
    """Closest monitor in direction (left|right|up|down) with axis overlap."""
    if not mons or idx < 0 or idx >= len(mons):
        return None
    cur = mons[idx].get_geometry()
    cx, cy = int(cur.x), int(cur.y)
    cw, ch = int(cur.width), int(cur.height)
    best: tuple[int, Gdk.Monitor] | None = None
    best_dist: float | None = None

    for i, mon in enumerate(mons):
        if i == idx:
            continue
        g = mon.get_geometry()
        gx, gy = int(g.x), int(g.y)
        gw, gh = int(g.width), int(g.height)
        overlap_x = min(cx + cw, gx + gw) - max(cx, gx)
        overlap_y = min(cy + ch, gy + gh) - max(cy, gy)

        dist: float | None = None
        if direction == "right" and gx >= cx + cw - 2 and overlap_y > 0:
            dist = float(gx - (cx + cw))
        elif direction == "left" and gx + gw <= cx + 2 and overlap_y > 0:
            dist = float(cx - (gx + gw))
        elif direction == "down" and gy >= cy + ch - 2 and overlap_x > 0:
            dist = float(gy - (cy + ch))
        elif direction == "up" and gy + gh <= cy + 2 and overlap_x > 0:
            dist = float(cy - (gy + gh))

        if dist is None:
            continue
        if best_dist is None or dist < best_dist:
            best_dist = dist
            best = (i, mon)
    return best


def focus_niri_output(connector: str) -> bool:
    """Move niri seat focus to ``connector`` so layer-shell can grab the keyboard.

    Best-effort: no-op when niri is missing or the name is empty.
    """
    name = (connector or "").strip()
    if not name:
        return False
    niri = shutil.which("niri")
    if not niri:
        return False
    try:
        proc = subprocess.run(
            [niri, "msg", "action", "focus-monitor", name],
            capture_output=True,
            text=True,
            timeout=1.0,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return False
    return proc.returncode == 0
