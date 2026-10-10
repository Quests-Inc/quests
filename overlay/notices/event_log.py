"""Persistent bottom-right event log (backed by /api/quest-log)."""

from __future__ import annotations

from datetime import datetime

from gi.repository import Gdk, Gtk, Pango
from gi.repository import Gtk4LayerShell as LayerShell

from ..stylepacks import active_pack, build_minor_log_css
from .minor import MINOR_CHANGE
from .shared import SIGNIFICANCE_LABEL_RU, clear_box, clickthrough

MINOR_LOG_MAX = 64
# Dense one-line rows (9pt); pad/spacing kept tight so capacity fills the panel.
_MINOR_LOG_LINE_PX = 14
_MINOR_LOG_PAD_PX = 8
_MINOR_LOG_SPACING_PX = 1

# Labels for durable /api/quest-log rows (majors + minors).
LOG_KIND_LABEL = {
    **MINOR_CHANGE,
    "quest_created": "Создано",
    "quest_appeared": "Появилось",
    "quest_started": "Началось",
    "quest_completed": "Завершено",
    "quest_failed": "Провалено",
    "quest_expired": "Просрочено",
}


def _looks_like_progress_label(text: str) -> bool:
    """True for progress_label noise like ``2 / 5`` or ``0/3``."""
    t = text.strip()
    if not t:
        return False
    compact = t.replace(" ", "")
    if "/" not in compact:
        return False
    left, _, right = compact.partition("/")
    return left.isdigit() and right.isdigit()


def format_log_message(kind: str, detail: str = "") -> str:
    """One-line action after the title: ``создано задание``, ``изменено: …``."""
    detail = (detail or "").strip()
    kind = (kind or "quest_updated").strip()

    if kind == "quest_created":
        return "создано задание"
    if kind == "quest_appeared":
        if detail and not _looks_like_progress_label(detail):
            return f"появилось задание ({detail})"
        return "появилось задание"
    if kind == "quest_started":
        return "началось задание"
    if kind == "quest_completed":
        return "завершено"
    if kind == "quest_failed":
        return "провалено"
    if kind == "quest_expired":
        return "просрочено"
    if kind == "quest_deleted":
        return "удалено"
    if kind == "quest_updated":
        if detail and not _looks_like_progress_label(detail):
            # Avoid double prefix if server already sent «изменено: …».
            low = detail.lower()
            if low.startswith("изменено"):
                return detail
            return f"изменено: {detail}"
        return "изменено"
    if kind == "step_completed":
        step = detail.split(" (", 1)[0].strip() if detail else ""
        return f"выполнен шаг: {step}" if step else "выполнен шаг"
    if kind == "step_reopened":
        return f"шаг открыт снова: {detail}" if detail else "шаг открыт снова"
    if kind == "step_progress":
        return f"прогресс: {detail}" if detail else "прогресс"
    if kind == "status_changed":
        return f"статус: {detail}" if detail else "изменён статус"
    if kind == "pin_changed":
        low = detail.lower()
        if "unpin" in low or "откреп" in low:
            return "откреплено"
        if "pin" in low or "закреп" in low:
            return "закреплено"
        return "закрепление изменено"
    if detail and not _looks_like_progress_label(detail):
        return detail
    return LOG_KIND_LABEL.get(kind, kind.replace("_", " ")).lower()


def format_log_ts(raw: object) -> str:
    """API ``at`` (UTC ISO) → local HH:MM:SS."""
    if raw is None or raw == "":
        return datetime.now().strftime("%H:%M:%S")
    text = str(raw).strip()
    try:
        if text.endswith("Z"):
            text = text[:-1] + "+00:00"
        dt = datetime.fromisoformat(text)
        if dt.tzinfo is not None:
            dt = dt.astimezone()
        return dt.strftime("%H:%M:%S")
    except ValueError:
        if len(text) >= 8 and text[2] == ":" and text[5] == ":":
            return text[:8]
        return datetime.now().strftime("%H:%M:%S")


class EventLogHost:
    """Persistent bottom-right event log (backed by /api/quest-log)."""

    def __init__(self, app: Gtk.Application) -> None:
        self._app = app
        self._entries: list[dict] = []  # newest first (API order)
        self._fp: tuple = ()
        self._bg_mode = "full"
        self._bg_alpha = 0.72
        self._text_alpha = 0.92
        self._width = 520
        self._height = 280
        self._css = Gtk.CssProvider()
        display = Gdk.Display.get_default()
        if display is not None:
            Gtk.StyleContext.add_provider_for_display(
                display, self._css, Gtk.STYLE_PROVIDER_PRIORITY_USER
            )
        self._window = self._build()
        self._apply_look()
        self._apply_size()

    def capacity(self) -> int:
        """How many one-line rows fit — slightly overestimate so top may clip."""
        line = _MINOR_LOG_LINE_PX
        spacing = _MINOR_LOG_SPACING_PX
        avail = max(0, int(self._height) - _MINOR_LOG_PAD_PX)
        n = (avail + spacing) // (line + spacing) if (line + spacing) else 1
        # +2: prefer clipping at the top edge over empty filler.
        return max(1, min(MINOR_LOG_MAX, int(n) + 2))

    def _build(self) -> Gtk.ApplicationWindow:
        window = Gtk.ApplicationWindow(application=self._app, title="Quests Event Log")
        window.set_decorated(False)
        window.set_default_size(520, 280)
        window.add_css_class("minor-log-window")
        LayerShell.init_for_window(window)
        LayerShell.set_namespace(window, "quests-minor-log")
        LayerShell.set_layer(window, LayerShell.Layer.TOP)
        LayerShell.set_anchor(window, LayerShell.Edge.BOTTOM, True)
        LayerShell.set_anchor(window, LayerShell.Edge.RIGHT, True)
        LayerShell.set_margin(window, LayerShell.Edge.BOTTOM, 24)
        LayerShell.set_margin(window, LayerShell.Edge.RIGHT, 24)
        LayerShell.set_keyboard_mode(window, LayerShell.KeyboardMode.NONE)
        LayerShell.set_exclusive_zone(window, 0)

        self._root = Gtk.Box(
            orientation=Gtk.Orientation.VERTICAL,
            spacing=_MINOR_LOG_SPACING_PX,
        )
        self._root.add_css_class("minor-log")
        self._root.set_halign(Gtk.Align.FILL)
        self._root.set_valign(Gtk.Align.FILL)
        self._root.set_hexpand(True)
        self._root.set_vexpand(True)
        self._root.set_overflow(Gtk.Overflow.HIDDEN)
        window.set_child(self._root)
        clickthrough(window)
        return window

    def set_look(
        self,
        *,
        bg_mode: str | None = None,
        bg_alpha: float | None = None,
        text_alpha: float | None = None,
        width: int | None = None,
        height: int | None = None,
        line_mode: str | None = None,
        style_pack: str | None = None,
    ) -> None:
        rebuild = False
        if bg_mode is not None:
            key = str(bg_mode).strip().lower()
            self._bg_mode = "full" if key in {"full", "panel", "solid"} else "chips"
        if bg_alpha is not None:
            try:
                self._bg_alpha = max(0.0, min(1.0, float(bg_alpha)))
            except (TypeError, ValueError):
                pass
        if text_alpha is not None:
            try:
                self._text_alpha = max(0.0, min(1.0, float(text_alpha)))
            except (TypeError, ValueError):
                pass
        if width is not None:
            try:
                w = max(280, min(1200, int(width)))
            except (TypeError, ValueError):
                w = self._width
            if w != self._width:
                self._width = w
                rebuild = True
        if height is not None:
            try:
                h = max(100, min(1200, int(height)))
            except (TypeError, ValueError):
                h = self._height
            if h != self._height:
                self._height = h
                rebuild = True
        # line_mode ignored — log is always single-line clip.
        _ = line_mode
        self._apply_look(style_pack=style_pack)
        self._apply_size()
        if rebuild:
            self._rebuild()

    def _apply_size(self) -> None:
        w, h = self._width, self._height
        self._window.set_default_size(w, h)
        self._window.set_size_request(w, h)
        self._root.set_size_request(w, h)

    def _apply_look(self, *, style_pack: str | None = None) -> None:
        self._css.load_from_string(
            build_minor_log_css(
                mode=self._bg_mode,
                bg_alpha=self._bg_alpha,
                text_alpha=self._text_alpha,
                width=self._width,
                height=self._height,
                name=style_pack or active_pack(),
            )
        )

    def load_from_api(self, rows: list[dict]) -> None:
        """Replace panel contents from /api/quest-log (newest first from API)."""
        fp = tuple(int(r["id"]) for r in rows if r.get("id") is not None)
        if fp == self._fp and self._entries:
            self._apply_size()
            self._window.present()
            return
        self._fp = fp
        parsed: list[dict] = []
        for row in rows[:MINOR_LOG_MAX]:
            kind = str(row.get("kind") or "quest_updated")
            detail = (row.get("detail") or "").strip()
            sig = str(row.get("significance") or "common").strip().lower()
            if sig not in SIGNIFICANCE_LABEL_RU:
                sig = "common"
            parsed.append(
                {
                    "ts": format_log_ts(row.get("at")),
                    "kind": kind,
                    "sig": sig,
                    "title": (row.get("title") or "—").strip() or "—",
                    "message": format_log_message(kind, detail),
                }
            )
        self._entries = parsed  # newest first
        self._rebuild()
        self._window.present()

    def hide(self) -> None:
        self._window.hide()

    def clear(self) -> None:
        self._entries.clear()
        self._fp = ()
        clear_box(self._root)
        self._window.hide()

    def _line_budgets(self) -> tuple[int, int, int]:
        """Pixel content width + title/msg char caps from panel width."""
        # CSS padding ~10–12px each side + row border/pad.
        content_w = max(140, int(self._width) - 28)
        # ~9pt mono-ish average glyph width.
        avg = 7
        cols = max(20, content_w // avg)
        # [HH:MM:SS]_ + " - " ≈ 14 cols reserved.
        reserved = 14
        title_max = max(8, min(28, (cols - reserved) // 3))
        msg_chars = max(8, cols - reserved - title_max)
        return content_w, title_max, msg_chars

    def _rebuild(self) -> None:
        clear_box(self._root)
        self._apply_size()
        content_w, title_max, msg_chars = self._line_budgets()
        cap = self.capacity()

        # Always bottom-align: when over capacity, excess clips at the top.
        filler = Gtk.Box()
        filler.set_vexpand(True)
        filler.set_hexpand(True)
        self._root.append(filler)

        if not self._entries:
            empty = Gtk.Label(label="нет событий", xalign=0)
            empty.add_css_class("minor-log__empty")
            empty.set_halign(Gtk.Align.START)
            empty.set_valign(Gtk.Align.END)
            self._root.append(empty)
            return

        # Newest at bottom; take newest `cap`, reverse to oldest→newest.
        visible = list(reversed(self._entries[:cap]))

        for entry in visible:
            row = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=0)
            row.add_css_class("minor-log__row")
            row.add_css_class(f"minor-log__row--{entry['kind']}")
            row.set_halign(Gtk.Align.START)
            row.set_hexpand(False)
            row.set_vexpand(False)
            row.set_size_request(content_w, -1)

            ts = Gtk.Label(label=f"[{entry['ts']}] ", xalign=0)
            ts.add_css_class("minor-log__ts")
            ts.set_halign(Gtk.Align.START)

            title = Gtk.Label(label=entry["title"], xalign=0)
            title.add_css_class("minor-log__title")
            title.add_css_class(f"minor-log__title--{entry['sig']}")
            title.set_halign(Gtk.Align.START)
            title.set_ellipsize(Pango.EllipsizeMode.END)
            title.set_max_width_chars(title_max)

            sep = Gtk.Label(label=" - ", xalign=0)
            sep.add_css_class("minor-log__sep")

            msg = Gtk.Label(label=entry["message"], xalign=0)
            msg.add_css_class("minor-log__msg")
            msg.add_css_class(f"minor-log__msg--{entry['kind']}")
            msg.set_halign(Gtk.Align.START)
            msg.set_hexpand(True)
            msg.set_wrap(False)
            msg.set_ellipsize(Pango.EllipsizeMode.END)
            msg.set_max_width_chars(msg_chars)

            row.append(ts)
            row.append(title)
            row.append(sep)
            row.append(msg)
            self._root.append(row)


# Back-compat alias used by older call sites / mental model.
MinorLogHost = EventLogHost
