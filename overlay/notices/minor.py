"""Minor (bottom-right) ephemeral toast host."""

from __future__ import annotations

from collections import deque

from gi.repository import GLib, Gtk
from gi.repository import Gtk4LayerShell as LayerShell

from .shared import animate_opacity, clear_box, clickthrough, pack_timing

MINOR_CHANGE = {
    "step_completed": "Шаг выполнен",
    "status_changed": "Статус изменён",
    "quest_deleted": "Удалено",
    "step_progress": "Прогресс",
    "pin_changed": "Закрепление",
    "quest_updated": "Обновлено",
}


class MinorHost:
    def __init__(self, app: Gtk.Application) -> None:
        self._app = app
        self._queue: deque[dict] = deque()
        self._busy = False
        self._window = self._build()

    def _build(self) -> Gtk.ApplicationWindow:
        window = Gtk.ApplicationWindow(application=self._app, title="Quests Minor")
        window.set_decorated(False)
        window.set_default_size(320, -1)
        LayerShell.init_for_window(window)
        LayerShell.set_namespace(window, "quests-minor")
        LayerShell.set_layer(window, LayerShell.Layer.TOP)
        LayerShell.set_anchor(window, LayerShell.Edge.BOTTOM, True)
        LayerShell.set_anchor(window, LayerShell.Edge.RIGHT, True)
        LayerShell.set_margin(window, LayerShell.Edge.BOTTOM, 24)
        LayerShell.set_margin(window, LayerShell.Edge.RIGHT, 24)
        LayerShell.set_keyboard_mode(window, LayerShell.KeyboardMode.NONE)
        LayerShell.set_exclusive_zone(window, 0)

        self._slot = Gtk.Box(orientation=Gtk.Orientation.VERTICAL)
        window.set_child(self._slot)
        clickthrough(window)
        return window

    def enqueue(self, event: dict) -> None:
        self._queue.append(event)
        self._pump()

    def hide(self) -> None:
        self._queue.clear()
        self._busy = False
        clear_box(self._slot)
        self._window.hide()

    def _pump(self) -> None:
        if self._busy or not self._queue:
            return
        self._busy = True
        self._show(self._queue.popleft())

    def _show(self, event: dict) -> None:
        clear_box(self._slot)
        kind = event.get("kind") or "quest_updated"
        *_, fade_in, hold, fade_out = pack_timing()

        card = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=0)
        card.add_css_class("minor")
        card.add_css_class(f"minor--{kind}")
        card.set_opacity(0.0)

        title = Gtk.Label(label=event.get("title") or "—", xalign=0)
        title.add_css_class("minor__title")
        title.set_wrap(True)
        change = Gtk.Label(
            label=MINOR_CHANGE.get(kind, kind.replace("_", " ")),
            xalign=0,
        )
        change.add_css_class("minor__change")
        card.append(title)
        card.append(change)

        detail = (event.get("detail") or "").strip()
        if detail:
            det = Gtk.Label(label=detail, xalign=0)
            det.add_css_class("minor__detail")
            det.set_wrap(True)
            card.append(det)

        self._slot.append(card)
        self._window.present()

        def after_in() -> None:
            def after_hold() -> bool:
                def after_out() -> None:
                    clear_box(self._slot)
                    self._busy = False
                    if not self._queue:
                        self._window.hide()
                    else:
                        self._pump()

                animate_opacity(card, 1.0, 0.0, fade_out, done=after_out)
                return False

            GLib.timeout_add(hold, after_hold)

        animate_opacity(card, 0.0, 1.0, fade_in, done=after_in)
