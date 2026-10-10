"""Click-through / interactive input region helpers for layer-shell surfaces."""

from __future__ import annotations

import cairo
from gi.repository import Gtk
from gi.repository import Gtk4LayerShell as LayerShell


def _surface_size(window: Gtk.Window) -> tuple[int, int] | None:
    surface = window.get_surface()
    if surface is None:
        return None
    w = int(surface.get_width() or 0)
    h = int(surface.get_height() or 0)
    if w <= 0 or h <= 0:
        w = max(w, int(window.get_width() or 0))
        h = max(h, int(window.get_height() or 0))
    if w <= 0 or h <= 0:
        alloc = window.get_allocation()
        w = max(w, int(alloc.width or 0))
        h = max(h, int(alloc.height or 0))
    if w <= 0 or h <= 0:
        return None
    return w, h


def apply_hud_input_mode(window: Gtk.Window, interactive: bool) -> bool:
    """Apply input region. Returns False if surface/size not ready yet."""
    surface = window.get_surface()
    if surface is None:
        return False

    if interactive:
        size = _surface_size(window)
        if size is None:
            return False
        w, h = size
        # Explicit full rect (Wayland): more reliable than None after an empty region.
        surface.set_input_region(cairo.Region(cairo.RectangleInt(0, 0, w, h)))
        # EXCLUSIVE: Esc/Space/arrows work right after toggle without a prior click.
        LayerShell.set_keyboard_mode(window, LayerShell.KeyboardMode.EXCLUSIVE)
        return True

    LayerShell.set_keyboard_mode(window, LayerShell.KeyboardMode.NONE)
    # Empty region ⇒ click-through.
    surface.set_input_region(cairo.Region())
    return True
