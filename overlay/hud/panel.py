from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass

from gi.repository import GLib, Gtk

from .deadline import format_remaining, is_urgent, remaining_seconds, timer_tone

# Persist settings ScrolledWindow Y across HUD rebuilds (chip/slider changes).
_settings_scroll_y: float = 0.0
from .monitors import list_monitors

# Header control glyphs (theme-independent).
ICON_FOLD = "−"
ICON_GEAR = "⚙"
ICON_LIST = "☰"


@dataclass
class HudStep:
    title: str
    progress: str


@dataclass
class HudQuest:
    quest_id: int | None
    title: str
    steps: list[HudStep]
    timer_label: str | None = None
    timer_tone: str | None = None  # green | orange | red | overdue
    deadline_at: object | None = None
    duration_seconds: int | None = None
    overdue: bool = False


@dataclass
class TimerBinding:
    """Live countdown chip — update in place without rebuilding the HUD tree."""

    label: Gtk.Label
    deadline_at: object
    duration_seconds: int | None
    tone: str | None = None


def _open_steps(quest: dict) -> list[HudStep]:
    steps: list[HudStep] = []
    for step in quest.get("steps") or []:
        cur = int(step.get("progress_current") or 0)
        total = max(1, int(step.get("progress_total") or 1))
        if step.get("done") or cur >= total:
            continue
        steps.append(HudStep(title=step.get("title") or "?", progress=f"{cur} / {total}"))
    return steps


def _with_timer(quest: dict, steps: list[HudStep]) -> HudQuest:
    qid = quest.get("id")
    rem = None
    deadline = quest.get("deadline_at")
    duration = quest.get("duration_seconds")
    status = str(quest.get("status") or "")
    overdue = status == "expired"
    if deadline:
        rem = remaining_seconds(deadline)
        if rem is not None and int(rem) <= 0:
            overdue = True
            rem = None
    tone = None
    timer_label = None
    if rem is not None:
        tone = timer_tone(deadline, duration)
        timer_label = format_remaining(int(rem))
    elif overdue:
        tone = "overdue"
        timer_label = "просрочено"
    return HudQuest(
        quest_id=int(qid) if qid is not None else None,
        title=quest.get("title") or "?",
        steps=steps,
        timer_label=timer_label,
        timer_tone=tone,
        deadline_at=deadline if rem is not None else None,
        duration_seconds=int(duration) if duration is not None else None,
        overdue=overdue,
    )


def split_hud_quests(
    items: list[dict],
    *,
    category_slug: str | None = None,
) -> tuple[list[HudQuest], list[HudQuest], list[HudQuest]]:
    """Pinned → urgent → category (active/expired). Earlier lanes win on overlap."""
    favorites: list[HudQuest] = []
    urgent: list[HudQuest] = []
    category: list[HudQuest] = []
    taken: set[int] = set()
    slug = (category_slug or "").strip().lower() or None
    # "Never in the HUD" (the crossed-out pin) beats every lane below.
    items = [q for q in items if not q.get("overlay_hidden")]

    for q in items:
        if q.get("status") in {"completed", "failed", "archived", "frozen"}:
            continue
        steps = _open_steps(q)
        if not steps:
            continue
        entry = _with_timer(q, steps)
        if q.get("pinned"):
            favorites.append(entry)
            if entry.quest_id is not None:
                taken.add(entry.quest_id)

    for q in items:
        if q.get("status") in {"completed", "failed", "archived", "frozen"}:
            continue
        qid = q.get("id")
        if qid is not None and int(qid) in taken:
            continue
        steps = _open_steps(q)
        if not steps:
            continue
        urgent_flag = q.get("urgent")
        if urgent_flag is None:
            urgent_flag = is_urgent(q.get("deadline_at"), q.get("duration_seconds"))
        status = str(q.get("status") or "")
        # Overdue / expired: same HUD lane as near-deadline (API may lag expire).
        if status == "expired":
            urgent_flag = True
        else:
            rem = remaining_seconds(q.get("deadline_at"))
            if rem is not None and int(rem) <= 0:
                urgent_flag = True
        if not urgent_flag:
            continue
        entry = _with_timer(q, steps)
        urgent.append(entry)
        if entry.quest_id is not None:
            taken.add(entry.quest_id)

    if slug:
        for q in items:
            status = str(q.get("status") or "")
            if status not in {"active", "expired"}:
                continue
            qid = q.get("id")
            if qid is not None and int(qid) in taken:
                continue
            q_slug = str(q.get("category_slug") or "").strip().lower()
            if q_slug != slug:
                continue
            steps = _open_steps(q)
            if not steps:
                continue
            category.append(_with_timer(q, steps))

    return favorites, urgent, category


def resolve_hud_category(
    categories: list[dict],
    preferred: str | None,
) -> tuple[str, str, list[tuple[str, str]]]:
    """Pick slug/label and option list for settings / cycling.

    Returns (slug, label, options[(slug, label)]). Empty categories → ("", "", []).
    """
    options: list[tuple[str, str]] = []
    for cat in categories:
        s = str(cat.get("slug") or "").strip()
        if not s:
            continue
        label = str(cat.get("label") or s).strip() or s
        options.append((s, label))
    if not options:
        return "", "", []
    want = (preferred or "").strip().lower()
    for s, label in options:
        if s.lower() == want:
            return s, label, options
    s0, label0 = options[0]
    return s0, label0, options


def cycle_hud_category(
    categories: list[dict],
    current: str | None,
    *,
    delta: int,
) -> str:
    """Cycle category slug by delta (±1). Returns new slug (or "")."""
    slug, _label, options = resolve_hud_category(categories, current)
    if not options:
        return ""
    slugs = [s for s, _ in options]
    try:
        idx = next(i for i, s in enumerate(slugs) if s.lower() == slug.lower())
    except StopIteration:
        idx = 0
    return slugs[(idx + int(delta)) % len(slugs)]


def _chip(text: str, css_class: str) -> Gtk.Label:
    lbl = Gtk.Label(label=text)
    lbl.set_halign(Gtk.Align.END)
    lbl.set_xalign(1.0)
    lbl.add_css_class("hud-chip")
    lbl.add_css_class(css_class)
    return lbl


def _hud_row(*children: Gtk.Widget, extra_classes: tuple[str, ...] = ()) -> Gtk.Box:
    """Right-aligned content plate for passthrough chips.

    Vertical Gtk.Box gives children the full cross-size (parent width). If the
    painted ``.hud-row`` is that child, short lines become long ghost bars.
    Outer stretcher has no background; only the inner plate is painted.
    """
    outer = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=0)
    outer.set_halign(Gtk.Align.FILL)
    outer.set_hexpand(True)

    pad = Gtk.Box()
    pad.set_hexpand(True)
    pad.set_halign(Gtk.Align.FILL)

    plate = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=8)
    plate.add_css_class("hud-row")
    for cls in extra_classes:
        plate.add_css_class(cls)
    plate.set_halign(Gtk.Align.END)
    plate.set_hexpand(False)
    for child in children:
        plate.append(child)

    outer.append(pad)
    outer.append(plate)
    return outer


def _rule(*, heavy: bool = False) -> Gtk.Box:
    rule = Gtk.Box()
    rule.add_css_class("section-rule")
    if heavy:
        rule.add_css_class("section-rule--heavy")
    # Hug CSS min-width; never hexpand — that stretches the layer-shell surface
    # full-output-wide in passthrough and leaves a ghost separator bar.
    rule.set_halign(Gtk.Align.END)
    rule.set_hexpand(False)
    return rule


def apply_timer_bindings(bindings: list[TimerBinding]) -> bool:
    """Update countdown chips in place. True → caller should rebuild HUD (expired)."""
    needs_rebuild = False
    for binding in bindings:
        rem = remaining_seconds(binding.deadline_at)
        if rem is None or rem <= 0:
            needs_rebuild = True
            continue
        text = format_remaining(int(rem))
        tone = timer_tone(binding.deadline_at, binding.duration_seconds) or "red"
        if binding.label.get_label() != text:
            binding.label.set_label(text)
        if binding.tone != tone:
            for old in ("green", "orange", "red", "overdue"):
                binding.label.remove_css_class(f"quest-timer--{old}")
            binding.label.add_css_class(f"quest-timer--{tone}")
            binding.tone = tone
    return needs_rebuild


def _append_quest_section(
    root: Gtk.Box,
    quest: HudQuest,
    *,
    interactive: bool,
    on_open_quest: Callable[[int], None] | None,
    timers: list[TimerBinding],
) -> None:
    section = Gtk.Box(
        orientation=Gtk.Orientation.VERTICAL,
        spacing=0 if not interactive else 4,
    )
    section.add_css_class("hud-section")
    section.set_halign(Gtk.Align.END)
    section.set_hexpand(False)

    chips: list[Gtk.Widget] = []

    if quest.timer_label and quest.deadline_at is not None:
        tone = quest.timer_tone or "red"
        timer = _chip(quest.timer_label, "quest-timer")
        timer.add_css_class(f"quest-timer--{tone}")
        chips.append(timer)
        timers.append(
            TimerBinding(
                label=timer,
                deadline_at=quest.deadline_at,
                duration_seconds=quest.duration_seconds,
                tone=tone,
            )
        )
    elif quest.timer_label:
        tone = quest.timer_tone or ("overdue" if quest.overdue else "red")
        timer = _chip(quest.timer_label, "quest-timer")
        timer.add_css_class(f"quest-timer--{tone}")
        chips.append(timer)

    if interactive and on_open_quest is not None and quest.quest_id is not None:
        title_btn = Gtk.Button(label=quest.title)
        title_btn.add_css_class("hud-chip")
        title_btn.add_css_class("section-title")
        title_btn.add_css_class("section-title-btn")
        title_btn.set_halign(Gtk.Align.END)
        title_btn.set_tooltip_text("Открыть в задачах")
        title_btn.connect("clicked", lambda _b, qid=quest.quest_id: on_open_quest(qid))
        chips.append(title_btn)
    else:
        chips.append(_chip(quest.title, "section-title"))

    section.append(_hud_row(*chips))
    # Decorative rules only in interactive panel — in chips passthrough they
    # still allocate width (longest quest) and paint a ghost bar under titles.
    if interactive:
        section.append(_rule())

    show_progress = True
    try:
        from overlay.stylepacks import _load_pack, active_pack

        show_progress = bool(getattr(_load_pack(active_pack()), "SHOW_STEP_PROGRESS", True))
    except Exception:
        show_progress = True
    for step in quest.steps:
        chips = []
        if show_progress:
            chips.append(_chip(step.progress, "quest-progress"))
        chips.append(_chip(step.title, "quest-title"))
        section.append(_hud_row(*chips, extra_classes=("quest",)))

    root.append(section)


def _append_section_heading(root: Gtk.Box, text: str, *, interactive: bool) -> None:
    block = Gtk.Box(
        orientation=Gtk.Orientation.VERTICAL,
        spacing=0 if not interactive else 2,
    )
    block.add_css_class("hud-section")
    block.add_css_class("hud-section--lane")
    block.set_halign(Gtk.Align.END)
    block.set_hexpand(False)
    block.append(_hud_row(_chip(text, "section-heading")))
    root.append(block)


def _append_heavy_sep(root: Gtk.Box, *, interactive: bool = True) -> None:
    if not interactive:
        return
    sep = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=0)
    sep.set_halign(Gtk.Align.END)
    sep.append(_rule(heavy=True))
    root.append(sep)


def _icon_button(
    glyph: str,
    *,
    tooltip: str,
    css_extra: str,
    on_click: Callable[[], None],
) -> Gtk.Button:
    btn = Gtk.Button(label=glyph)
    btn.add_css_class("hud-icon-btn")
    btn.add_css_class(css_extra)
    btn.set_tooltip_text(tooltip)
    btn.set_valign(Gtk.Align.CENTER)
    btn.set_halign(Gtk.Align.CENTER)
    btn.set_can_focus(False)
    if hasattr(btn, "set_has_frame"):
        btn.set_has_frame(False)
    btn.connect("clicked", lambda _b: on_click())
    return btn


def _settings_label(text: str) -> Gtk.Label:
    lbl = Gtk.Label(label=text)
    lbl.add_css_class("hud-settings-label")
    lbl.set_halign(Gtk.Align.END)
    lbl.set_xalign(1.0)
    return lbl


def _settings_section(text: str) -> Gtk.Label:
    lbl = Gtk.Label(label=text)
    lbl.add_css_class("hud-settings-section")
    lbl.set_halign(Gtk.Align.END)
    lbl.set_xalign(1.0)
    return lbl


def _settings_sep() -> Gtk.Box:
    """Horizontal rule between settings sections."""
    sep = Gtk.Box()
    sep.add_css_class("hud-settings-sep")
    sep.set_halign(Gtk.Align.FILL)
    sep.set_hexpand(True)
    sep.set_size_request(-1, 1)
    return sep


def _settings_max_height(monitor_index: int) -> int:
    """Cap settings panel at half the current output height."""
    mons = list_monitors()
    if not mons:
        return 540
    idx = int(monitor_index) % len(mons)
    try:
        h = int(mons[idx].get_geometry().height)
    except Exception:
        h = 1080
    return max(240, h // 2)


def _block_scale_wheel(scale: Gtk.Scale) -> None:
    """Keep mouse-wheel for the settings ScrolledWindow, not the Scale."""
    ctl = Gtk.EventControllerScroll.new(
        Gtk.EventControllerScrollFlags.BOTH_AXES
        | Gtk.EventControllerScrollFlags.DISCRETE
    )
    ctl.set_propagation_phase(Gtk.PropagationPhase.CAPTURE)

    def _eat(_c: Gtk.EventControllerScroll, _dx: float, _dy: float) -> bool:
        return True

    ctl.connect("scroll", _eat)
    scale.add_controller(ctl)


def _alpha_scale(
    value: float,
    on_change: Callable[[float], None],
) -> Gtk.Scale:
    adj = Gtk.Adjustment(
        value=round(max(0.0, min(1.0, float(value))) * 100),
        lower=0,
        upper=100,
        step_increment=1,
        page_increment=5,
    )
    scale = Gtk.Scale(orientation=Gtk.Orientation.HORIZONTAL, adjustment=adj)
    scale.set_digits(0)
    scale.set_draw_value(True)
    scale.set_value_pos(Gtk.PositionType.RIGHT)
    scale.set_hexpand(True)
    scale.set_size_request(220, -1)
    scale.add_css_class("hud-settings-scale")
    scale.set_halign(Gtk.Align.FILL)
    _block_scale_wheel(scale)
    scale.connect("value-changed", lambda s: on_change(float(s.get_value()) / 100.0))
    return scale


def _int_scale(
    value: int,
    *,
    lower: int,
    upper: int,
    on_change: Callable[[int], None],
) -> Gtk.Scale:
    adj = Gtk.Adjustment(
        value=max(lower, min(upper, int(value))),
        lower=lower,
        upper=upper,
        step_increment=10,
        page_increment=40,
    )
    scale = Gtk.Scale(orientation=Gtk.Orientation.HORIZONTAL, adjustment=adj)
    scale.set_digits(0)
    scale.set_draw_value(True)
    scale.set_value_pos(Gtk.PositionType.RIGHT)
    scale.set_hexpand(True)
    scale.set_size_request(220, -1)
    scale.add_css_class("hud-settings-scale")
    scale.set_halign(Gtk.Align.FILL)
    _block_scale_wheel(scale)
    scale.connect("value-changed", lambda s: on_change(int(round(s.get_value()))))
    return scale


def _opt_slider(
    options: list[tuple[str, str]],
    active_id: str,
    on_pick: Callable[[str], None],
) -> Gtk.Box:
    """Horizontal exclusive option chips (monitor / style / bg mode)."""
    row = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=0)
    row.add_css_class("hud-opt-slider")
    row.set_halign(Gtk.Align.END)
    row.set_hexpand(True)

    for i, (oid, label) in enumerate(options):
        btn = Gtk.Button(label=label)
        btn.add_css_class("hud-opt")
        if oid == active_id:
            btn.add_css_class("hud-opt--on")
        if i == 0:
            btn.add_css_class("hud-opt--first")
        if i == len(options) - 1:
            btn.add_css_class("hud-opt--last")
        btn.set_hexpand(True)
        btn.set_halign(Gtk.Align.FILL)
        if hasattr(btn, "set_has_frame"):
            btn.set_has_frame(False)

        def _pick(_b: Gtk.Button, picked: str = oid) -> None:
            on_pick(picked)

        btn.connect("clicked", _pick)
        row.append(btn)
    return row


def _append_settings_panel(
    root: Gtk.Box,
    *,
    monitors: list[tuple[int, str]],
    monitor_index: int,
    style_pack_id: str,
    style_packs: list[tuple[str, str]],
    categories: list[tuple[str, str]],
    category_slug: str,
    bg_mode: str,
    bg_alpha: float,
    hud_text_alpha: float,
    toasts_major: bool,
    toasts_minor_mode: str,
    minor_bg_mode: str,
    minor_bg_alpha: float,
    minor_text_alpha: float,
    minor_log_width: int,
    minor_log_height: int,
    minor_log_line_mode: str,
    on_select_monitor: Callable[[int], None] | None,
    on_select_style: Callable[[str], None] | None,
    on_select_category: Callable[[str], None] | None,
    on_hud_look: Callable[[str, float, float], None] | None,
    on_major_toasts: Callable[[bool], None] | None,
    on_minor_toasts: Callable[[dict], None] | None,
) -> None:
    panel = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=10)
    panel.add_css_class("hud-settings-panel")
    panel.set_halign(Gtk.Align.END)
    panel.set_size_request(280, -1)

    mode = "full" if str(bg_mode).strip().lower() in {"full", "panel", "solid"} else "chips"
    alpha = max(0.0, min(1.0, float(bg_alpha)))
    hud_alpha = max(0.0, min(1.0, float(hud_text_alpha)))
    minor_mode = str(toasts_minor_mode or "toast").strip().lower()
    if minor_mode not in {"off", "toast", "log"}:
        minor_mode = "toast"
    m_bg_mode = (
        "full"
        if str(minor_bg_mode).strip().lower() in {"full", "panel", "solid"}
        else "chips"
    )
    m_bg_a = max(0.0, min(1.0, float(minor_bg_alpha)))
    m_text_a = max(0.0, min(1.0, float(minor_text_alpha)))
    m_width = max(280, min(1200, int(minor_log_width)))
    m_height = max(100, min(1200, int(minor_log_height)))
    _ = minor_log_line_mode  # log is always clip; kept for config compat
    on_off = [("1", "вкл"), ("0", "выкл")]
    bg_opts = [("chips", "выделение"), ("full", "полный")]

    # —— Общее ——
    panel.append(_settings_section("Общее"))
    panel.append(_settings_label("Монитор"))
    if monitors and on_select_monitor is not None:
        mon_opts = [(str(i), label) for i, label in monitors]
        active_mon = str(
            monitor_index if any(i == monitor_index for i, _ in monitors) else monitors[0][0]
        )

        def _pick_mon(oid: str) -> None:
            on_select_monitor(int(oid))

        panel.append(_opt_slider(mon_opts, active_mon, _pick_mon))
    else:
        panel.append(_chip("нет мониторов", "hint"))

    panel.append(_settings_label("Стиль"))
    if style_packs and on_select_style is not None:
        panel.append(_opt_slider(style_packs, style_pack_id, on_select_style))
    else:
        panel.append(_chip(style_pack_id, "hint"))

    # —— HUD ——
    panel.append(_settings_sep())
    panel.append(_settings_section("HUD"))
    panel.append(_settings_label("Раздел"))
    if categories and on_select_category is not None:
        active_cat = (
            category_slug
            if any(s == category_slug for s, _ in categories)
            else categories[0][0]
        )
        panel.append(_opt_slider(categories, active_cat, on_select_category))
        cat_hint = Gtk.Label(label="z / x — листать раздел")
        cat_hint.add_css_class("hint")
        cat_hint.add_css_class("hud-settings-hint")
        cat_hint.set_halign(Gtk.Align.END)
        cat_hint.set_xalign(1.0)
        panel.append(cat_hint)
    else:
        panel.append(_chip("нет разделов", "hint"))

    panel.append(_settings_label("Фон (passthrough)"))
    hud_look = {"mode": mode, "alpha": alpha, "text": hud_alpha}

    def _emit_hud_look(
        *,
        next_mode: str | None = None,
        next_alpha: float | None = None,
        next_text: float | None = None,
    ) -> None:
        if on_hud_look is None:
            return
        if next_mode is not None:
            hud_look["mode"] = next_mode
        if next_alpha is not None:
            hud_look["alpha"] = next_alpha
        if next_text is not None:
            hud_look["text"] = next_text
        on_hud_look(hud_look["mode"], hud_look["alpha"], hud_look["text"])

    def _pick_bg(oid: str) -> None:
        _emit_hud_look(next_mode=oid)

    panel.append(_opt_slider(bg_opts, mode, _pick_bg))
    panel.append(_settings_label("Альфа фона"))
    panel.append(_alpha_scale(alpha, lambda v: _emit_hud_look(next_alpha=v)))
    panel.append(_settings_label("Альфа текста"))
    panel.append(_alpha_scale(hud_alpha, lambda v: _emit_hud_look(next_text=v)))

    # —— Мажорные тосты ——
    panel.append(_settings_sep())
    panel.append(_settings_section("Мажорные тосты"))
    major_on = bool(toasts_major)

    def _pick_major(oid: str) -> None:
        if on_major_toasts is None:
            return
        on_major_toasts(oid == "1")

    panel.append(_opt_slider(on_off, "1" if major_on else "0", _pick_major))

    # —— Минорные тосты ——
    panel.append(_settings_sep())
    panel.append(_settings_section("Минорные тосты"))
    panel.append(_settings_label("Режим"))
    minor_opts = [("off", "выкл"), ("toast", "тост"), ("log", "лог")]
    minor_look: dict = {
        "mode": minor_mode,
        "bg": m_bg_mode,
        "bg_a": m_bg_a,
        "text_a": m_text_a,
        "width": m_width,
        "height": m_height,
        "line_mode": "clip",
    }

    def _emit_minor(**updates) -> None:
        if on_minor_toasts is None:
            return
        minor_look.update(updates)
        on_minor_toasts(dict(minor_look))

    panel.append(_opt_slider(minor_opts, minor_mode, lambda oid: _emit_minor(mode=oid)))
    panel.append(_settings_label("Выделение"))
    panel.append(_opt_slider(bg_opts, m_bg_mode, lambda oid: _emit_minor(bg=oid)))
    panel.append(_settings_label("Альфа фона"))
    panel.append(_alpha_scale(m_bg_a, lambda v: _emit_minor(bg_a=v)))
    panel.append(_settings_label("Альфа текста"))
    panel.append(_alpha_scale(m_text_a, lambda v: _emit_minor(text_a=v)))
    panel.append(_settings_label("Ширина лога"))
    panel.append(
        _int_scale(m_width, lower=280, upper=900, on_change=lambda v: _emit_minor(width=v))
    )
    panel.append(_settings_label("Высота лога"))
    panel.append(
        _int_scale(
            m_height, lower=120, upper=800, on_change=lambda v: _emit_minor(height=v)
        )
    )

    max_h = _settings_max_height(monitor_index)
    scroll = Gtk.ScrolledWindow()
    scroll.add_css_class("hud-settings-scroll")
    scroll.set_policy(Gtk.PolicyType.NEVER, Gtk.PolicyType.AUTOMATIC)
    # Scrollbar on the left (overlay — no exclusive gutter).
    scroll.set_placement(Gtk.CornerType.TOP_RIGHT)
    scroll.set_halign(Gtk.Align.END)
    scroll.set_hexpand(False)
    scroll.set_vexpand(False)
    if hasattr(scroll, "set_propagate_natural_height"):
        scroll.set_propagate_natural_height(True)
    if hasattr(scroll, "set_propagate_natural_width"):
        scroll.set_propagate_natural_width(True)
    if hasattr(scroll, "set_max_content_height"):
        scroll.set_max_content_height(max_h)
    scroll.set_size_request(280, -1)
    scroll.set_child(panel)

    vadj = scroll.get_vadjustment()
    if vadj is not None:

        def _remember(_a: Gtk.Adjustment) -> None:
            global _settings_scroll_y
            _settings_scroll_y = float(vadj.get_value())

        vadj.connect("value-changed", _remember)

        def _restore_y() -> bool:
            upper = float(vadj.get_upper() - vadj.get_page_size())
            vadj.set_value(max(0.0, min(float(_settings_scroll_y), max(0.0, upper))))
            return False

        GLib.idle_add(_restore_y)

    root.append(scroll)


def build_hud(
    favorites: list[HudQuest],
    urgent: list[HudQuest] | None = None,
    category: list[HudQuest] | None = None,
    *,
    interactive: bool = False,
    collapsed: bool = False,
    settings_open: bool = False,
    monitors: list[tuple[int, str]] | None = None,
    monitor_index: int = 0,
    style_pack_id: str = "fantasy",
    style_packs: list[tuple[str, str]] | None = None,
    categories: list[tuple[str, str]] | None = None,
    category_slug: str = "",
    category_label: str = "",
    passthrough_bg_mode: str = "chips",
    passthrough_bg_alpha: float = 0.6,
    hud_text_alpha: float = 0.92,
    toasts_major: bool = True,
    toasts_minor_mode: str = "toast",
    minor_bg_mode: str = "full",
    minor_bg_alpha: float = 0.72,
    minor_text_alpha: float = 0.92,
    minor_log_width: int = 520,
    minor_log_height: int = 280,
    minor_log_line_mode: str = "clip",
    on_select_monitor: Callable[[int], None] | None = None,
    on_select_style: Callable[[str], None] | None = None,
    on_select_category: Callable[[str], None] | None = None,
    on_hud_look: Callable[[str, float, float], None] | None = None,
    on_major_toasts: Callable[[bool], None] | None = None,
    on_minor_toasts: Callable[[dict], None] | None = None,
    on_toggle_collapsed: Callable[[], None] | None = None,
    on_toggle_settings: Callable[[], None] | None = None,
    on_prepare_drag_handle: Callable[[Gtk.Widget], None] | None = None,
    on_open_quest: Callable[[int], None] | None = None,
) -> tuple[Gtk.Widget, Gtk.Widget | None, list[TimerBinding]]:
    urgent = urgent or []
    category = category or []
    monitors = monitors or []
    style_packs = style_packs or []
    categories = categories or []
    timers: list[TimerBinding] = []
    show_settings = bool(interactive and settings_open and not collapsed)

    root = Gtk.Box(
        orientation=Gtk.Orientation.VERTICAL,
        spacing=0 if not interactive else 14,
    )
    root.add_css_class("hud")
    if interactive:
        root.add_css_class("hud--interactive")
    if collapsed:
        root.add_css_class("hud--collapsed")
    if show_settings:
        root.add_css_class("hud--settings")
    root.set_halign(Gtk.Align.END)

    header = Gtk.Box(
        orientation=Gtk.Orientation.VERTICAL,
        spacing=0,
    )
    header.add_css_class("hud-header")
    header.set_halign(Gtk.Align.END)

    hotspot: Gtk.Widget | None = None
    controls: Gtk.Widget | None = None

    if collapsed:
        header.append(_hud_row(_chip("Задачи", "title")))
        root.append(header)
        return root, None, timers

    # Fold/gear sit as overlays at the window corner (like drag) — built after root.
    if interactive and (on_toggle_collapsed is not None or on_toggle_settings is not None):
        controls = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=2)
        controls.add_css_class("hud-header-controls")
        controls.set_halign(Gtk.Align.END)
        controls.set_valign(Gtk.Align.START)

        if on_toggle_collapsed is not None:
            fold_btn = _icon_button(
                ICON_FOLD,
                tooltip="Свернуть HUD (Backspace)",
                css_extra="hud-icon-btn--fold",
                on_click=on_toggle_collapsed,
            )
            controls.append(fold_btn)
            hotspot = fold_btn

        if on_toggle_settings is not None:
            gear_btn = _icon_button(
                ICON_LIST if show_settings else ICON_GEAR,
                tooltip="К квестам" if show_settings else "Настройки",
                css_extra="hud-icon-btn--gear",
                on_click=on_toggle_settings,
            )
            controls.append(gear_btn)
            hotspot = gear_btn

    # Reserve vertical room: fold/gear are overlaid flush to the corner above the title.
    if controls is not None:
        spacer = Gtk.Box()
        spacer.set_size_request(-1, 22)
        spacer.set_hexpand(False)
        spacer.set_halign(Gtk.Align.END)
        header.append(spacer)
    header.append(_hud_row(_chip("Настройки" if show_settings else "Задачи", "title")))

    root.append(header)

    if show_settings:
        _append_settings_panel(
            root,
            monitors=monitors,
            monitor_index=monitor_index,
            style_pack_id=style_pack_id,
            style_packs=style_packs,
            categories=categories,
            category_slug=category_slug,
            bg_mode=passthrough_bg_mode,
            bg_alpha=passthrough_bg_alpha,
            hud_text_alpha=hud_text_alpha,
            toasts_major=toasts_major,
            toasts_minor_mode=toasts_minor_mode,
            minor_bg_mode=minor_bg_mode,
            minor_bg_alpha=minor_bg_alpha,
            minor_text_alpha=minor_text_alpha,
            minor_log_width=minor_log_width,
            minor_log_height=minor_log_height,
            minor_log_line_mode=minor_log_line_mode,
            on_select_monitor=on_select_monitor,
            on_select_style=on_select_style,
            on_select_category=on_select_category,
            on_hud_look=on_hud_look,
            on_major_toasts=on_major_toasts,
            on_minor_toasts=on_minor_toasts,
        )
    elif not favorites and not urgent and not category:
        empty = Gtk.Box(
            orientation=Gtk.Orientation.VERTICAL,
            spacing=0 if not interactive else 4,
        )
        empty.add_css_class("hud-section")
        empty.set_halign(Gtk.Align.END)
        empty.set_hexpand(False)
        empty.append(_hud_row(_chip("Нет активных шагов", "hint")))
        root.append(empty)
    else:
        for quest in favorites:
            _append_quest_section(
                root,
                quest,
                interactive=interactive,
                on_open_quest=on_open_quest,
                timers=timers,
            )

        if favorites and urgent:
            _append_heavy_sep(root, interactive=interactive)

        for quest in urgent:
            _append_quest_section(
                root,
                quest,
                interactive=interactive,
                on_open_quest=on_open_quest,
                timers=timers,
            )

        # Category lane only when it has something to show (avoid orphan heading
        # + empty hint that still paints a stray passthrough bar).
        if category:
            if favorites or urgent:
                _append_heavy_sep(root, interactive=interactive)
            heading = (category_label or category_slug or "Раздел").strip()
            _append_section_heading(root, heading, interactive=interactive)
            for quest in category:
                _append_quest_section(
                    root,
                    quest,
                    interactive=interactive,
                    on_open_quest=on_open_quest,
                    timers=timers,
                )

    if interactive and (
        on_prepare_drag_handle is not None or controls is not None
    ):
        overlay = Gtk.Overlay()
        overlay.set_child(root)

        # Fold/gear: flush to top-right window corner (outside content padding).
        if controls is not None:
            overlay.add_overlay(controls)

        if on_prepare_drag_handle is not None:
            drag = Gtk.Button(label="⠿")
            drag.add_css_class("hud-drag")
            drag.set_tooltip_text("Перетащить HUD")
            drag.set_halign(Gtk.Align.START)
            drag.set_valign(Gtk.Align.START)
            drag.set_cursor_from_name("grab")
            drag.set_can_focus(False)
            if hasattr(drag, "set_has_frame"):
                drag.set_has_frame(False)
            on_prepare_drag_handle(drag)
            overlay.add_overlay(drag)

        return overlay, hotspot, timers

    return root, hotspot, timers
