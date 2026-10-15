//go:build quests_overlay

package hud

import (
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
)

// Hardware keycodes (X11 = linux evdev + 8) — layout-independent.
const (
	kcH = 43
	kcJ = 44
	kcK = 45
	kcL = 46
	kcS = 39
	kcZ = 52
	kcX = 53
)

func (h *Host) attachKeys() {
	ctrl := gtk.NewEventControllerKey()
	ctrl.SetPropagationPhase(gtk.PhaseCapture)
	ctrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		return h.onKey(keyval, keycode, state)
	})
	h.win.AddController(ctrl)
}

func (h *Host) onKey(keyval, keycode uint, state gdk.ModifierType) bool {
	if !h.interactive {
		return false
	}

	shift := state&gdk.ShiftMask != 0

	switch keyval {
	case gdk.KEY_Escape:
		if h.settingsOpen {
			h.settingsOpen = false
			h.repaint()
			return true
		}
		h.setInteractive(false)
		return true
	case gdk.KEY_BackSpace:
		h.setCollapsed(true)
		return true
	}

	// s — settings ↔ quests
	if keycode == kcS {
		h.toggleSettings()
		return true
	}

	if h.settingsOpen {
		return h.onSettingsKey(keyval, keycode, shift)
	}
	return h.onQuestsKey(keyval, keycode)
}

func (h *Host) onQuestsKey(keyval, keycode uint) bool {
	switch keyval {
	case gdk.KEY_space, gdk.KEY_KP_Space:
		h.cycleMonitor()
		return true
	}

	if keycode == kcZ {
		h.cycleCategory(-1)
		return true
	}
	if keycode == kcX {
		h.cycleCategory(1)
		return true
	}

	dx, dy := 0, 0
	step := shell.NudgeStep
	switch {
	case keyval == gdk.KEY_Left || keyval == gdk.KEY_KP_Left || keycode == kcH:
		dx = -step
	case keyval == gdk.KEY_Right || keyval == gdk.KEY_KP_Right || keycode == kcL:
		dx = step
	case keyval == gdk.KEY_Up || keyval == gdk.KEY_KP_Up || keycode == kcK:
		dy = -step
	case keyval == gdk.KEY_Down || keyval == gdk.KEY_KP_Down || keycode == kcJ:
		dy = step
	default:
		return false
	}

	mons := shell.ListMonitors(gdk.DisplayGetDefault())
	idx := shell.ResolveMonitorIndex(mons, h.cfg.MonitorConnector, h.cfg.MonitorIndex)
	var mon *gdk.Monitor
	if len(mons) > 0 {
		mon = mons[idx]
	}
	mt, mr, changed := shell.NudgeHUD(&h.win.Window, mon, dx, dy)
	if changed {
		h.cfg.MarginTop = mt
		h.cfg.MarginRight = mr
		h.persistMargins()
	}
	return true
}

func (h *Host) onSettingsKey(keyval, keycode uint, shift bool) bool {
	groups := buildSettingsGroups(h.cfg, h.categories)
	if len(groups) == 0 {
		return true
	}
	if h.settingsFocus < 0 {
		h.settingsFocus = 0
	}
	if h.settingsFocus >= len(groups) {
		h.settingsFocus = len(groups) - 1
	}

	// Tab / j/k / Up/Down — смена группы
	switch {
	case keyval == gdk.KEY_Tab && !shift, keyval == gdk.KEY_Down || keyval == gdk.KEY_KP_Down, keycode == kcJ:
		h.settingsFocus = (h.settingsFocus + 1) % len(groups)
		h.repaint()
		return true
	case keyval == gdk.KEY_Tab && shift, keyval == gdk.KEY_ISO_Left_Tab, keyval == gdk.KEY_Up || keyval == gdk.KEY_KP_Up, keycode == kcK:
		h.settingsFocus = (h.settingsFocus - 1 + len(groups)) % len(groups)
		h.repaint()
		return true
	}

	g := groups[h.settingsFocus]
	delta := 0
	switch {
	case keyval == gdk.KEY_Left || keyval == gdk.KEY_KP_Left || keycode == kcH, keyval == gdk.KEY_minus, keyval == gdk.KEY_KP_Subtract:
		delta = -1
	case keyval == gdk.KEY_Right || keyval == gdk.KEY_KP_Right || keycode == kcL, keyval == gdk.KEY_equal, keyval == gdk.KEY_plus, keyval == gdk.KEY_KP_Add:
		delta = 1
	case keyval == gdk.KEY_bracketleft:
		delta = -1
	case keyval == gdk.KEY_bracketright:
		delta = 1
	default:
		return false
	}

	h.applySettingsDelta(g, delta)
	return true
}

func (h *Host) applySettingsDelta(g settingsGroup, delta int) {
	switch g.ID {
	case "monitor":
		id := cycleChoice(g.Options, g.Active, delta)
		idx, _ := strconv.Atoi(id)
		h.selectMonitor(idx)
	case "style":
		_ = h.setStylePack(cycleChoice(g.Options, g.Active, delta))
	case "category":
		h.selectCategory(cycleChoice(g.Options, g.Active, delta))
	case "bg_mode":
		mode := cycleChoice(g.Options, g.Active, delta)
		h.setHUDLook(mode, h.cfg.PassthroughAlpha, h.cfg.HUDTextAlpha)
		h.repaint()
	case "bg_alpha":
		v := clampScale(g.Value+float64(delta)*g.Step, g.Min, g.Max) / 100
		h.setHUDLook(h.cfg.PassthroughBG, v, h.cfg.HUDTextAlpha)
		h.repaint()
	case "text_alpha":
		v := clampScale(g.Value+float64(delta)*g.Step, g.Min, g.Max) / 100
		h.setHUDLook(h.cfg.PassthroughBG, h.cfg.PassthroughAlpha, v)
		h.repaint()
	case "major":
		h.setMajorToasts(cycleChoice(g.Options, g.Active, delta) == "1")
	case "minor_mode":
		mode := cycleChoice(g.Options, g.Active, delta)
		h.setMinorToasts(mode, h.cfg.MinorBGMode, h.cfg.MinorBGAlpha, h.cfg.MinorTextAlpha, h.cfg.MinorLogWidth, h.cfg.MinorLogHeight)
		h.repaint()
	case "minor_bg":
		bg := cycleChoice(g.Options, g.Active, delta)
		h.setMinorToasts(h.cfg.ToastsMinorMode, bg, h.cfg.MinorBGAlpha, h.cfg.MinorTextAlpha, h.cfg.MinorLogWidth, h.cfg.MinorLogHeight)
		h.repaint()
	case "minor_bg_a":
		v := clampScale(g.Value+float64(delta)*g.Step, g.Min, g.Max) / 100
		h.setMinorToasts(h.cfg.ToastsMinorMode, h.cfg.MinorBGMode, v, h.cfg.MinorTextAlpha, h.cfg.MinorLogWidth, h.cfg.MinorLogHeight)
		h.repaint()
	case "minor_text_a":
		v := clampScale(g.Value+float64(delta)*g.Step, g.Min, g.Max) / 100
		h.setMinorToasts(h.cfg.ToastsMinorMode, h.cfg.MinorBGMode, h.cfg.MinorBGAlpha, v, h.cfg.MinorLogWidth, h.cfg.MinorLogHeight)
		h.repaint()
	case "minor_w":
		v := int(clampScale(g.Value+float64(delta)*g.Step, g.Min, g.Max))
		h.setMinorToasts(h.cfg.ToastsMinorMode, h.cfg.MinorBGMode, h.cfg.MinorBGAlpha, h.cfg.MinorTextAlpha, v, h.cfg.MinorLogHeight)
		h.repaint()
	case "minor_h":
		v := int(clampScale(g.Value+float64(delta)*g.Step, g.Min, g.Max))
		h.setMinorToasts(h.cfg.ToastsMinorMode, h.cfg.MinorBGMode, h.cfg.MinorBGAlpha, h.cfg.MinorTextAlpha, h.cfg.MinorLogWidth, v)
		h.repaint()
	}
}
