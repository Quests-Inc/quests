//go:build quests_overlay

package hud

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
)

var settingsScrollY float64

type opt struct {
	ID, Label string
}

func appendSettingsPanel(root *gtk.Box, cfg config.Config, cats []api.Category, focus int) {
	groups := buildSettingsGroups(cfg, cats)
	if len(groups) == 0 {
		root.Append(settingsHint("нет настроек"))
		return
	}
	if focus < 0 {
		focus = 0
	}
	if focus >= len(groups) {
		focus = len(groups) - 1
	}

	panel := gtk.NewBox(gtk.OrientationVertical, 8)
	panel.AddCSSClass("hud-settings-panel")
	panel.SetHAlign(gtk.AlignEnd)
	panel.SetSizeRequest(300, -1)

	hint := gtk.NewLabel("j/k · группа   h/l · значение   s · квесты")
	hint.AddCSSClass("hint")
	hint.AddCSSClass("hud-settings-hint")
	hint.SetHAlign(gtk.AlignEnd)
	hint.SetXAlign(1)
	panel.Append(hint)

	section := ""
	var focusRow *gtk.Box
	for i, g := range groups {
		sec := groupSection(g.ID)
		if sec != section {
			if section != "" {
				panel.Append(settingsSep())
			}
			panel.Append(settingsSection(sec))
			section = sec
		}

		row := gtk.NewBox(gtk.OrientationVertical, 2)
		row.AddCSSClass("hud-settings-row")
		if i == focus {
			row.AddCSSClass("hud-settings-row--focus")
			focusRow = row
		}

		prefix := "  "
		if i == focus {
			prefix = "▸ "
		}
		lbl := gtk.NewLabel(prefix + g.Label)
		lbl.AddCSSClass("hud-settings-label")
		if i == focus {
			lbl.AddCSSClass("hud-settings-label--focus")
		}
		lbl.SetHAlign(gtk.AlignEnd)
		lbl.SetXAlign(1)
		row.Append(lbl)

		val := gtk.NewLabel(formatGroupValue(g))
		val.AddCSSClass("hud-settings-value")
		if i == focus {
			val.AddCSSClass("hud-settings-value--focus")
		}
		val.SetHAlign(gtk.AlignEnd)
		val.SetXAlign(1)
		val.SetWrap(true)
		row.Append(val)

		panel.Append(row)
	}

	maxH := settingsMaxHeight(cfg.MonitorIndex)
	scroll := gtk.NewScrolledWindow()
	scroll.AddCSSClass("hud-settings-scroll")
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetPlacement(gtk.CornerTopRight)
	scroll.SetHAlign(gtk.AlignEnd)
	scroll.SetPropagateNaturalHeight(true)
	scroll.SetPropagateNaturalWidth(true)
	scroll.SetMaxContentHeight(maxH)
	scroll.SetSizeRequest(300, -1)
	scroll.SetChild(panel)

	if vadj := scroll.VAdjustment(); vadj != nil {
		vadj.ConnectValueChanged(func() {
			settingsScrollY = vadj.Value()
		})
		// After layout: keep the focused row in view (j/k can walk past the viewport).
		scrollSettingsToFocus(scroll, panel, focusRow)
	}
	root.Append(scroll)
}

func scrollSettingsToFocus(scroll *gtk.ScrolledWindow, panel *gtk.Box, focusRow *gtk.Box) {
	if focusRow == nil {
		return
	}
	try := func() bool {
		vadj := scroll.VAdjustment()
		if vadj == nil {
			return false
		}
		_, y, ok := focusRow.TranslateCoordinates(panel, 0, 0)
		if !ok {
			return true // not mapped yet — retry
		}
		h := float64(focusRow.AllocatedHeight())
		if h <= 0 {
			h = float64(focusRow.Height())
		}
		page := vadj.PageSize()
		if page <= 0 {
			return true
		}
		val := vadj.Value()
		pad := 8.0
		target := val
		if y-pad < val {
			target = y - pad
		} else if y+h+pad > val+page {
			target = y + h + pad - page
		}
		upper := vadj.Upper() - page
		if upper < 0 {
			upper = 0
		}
		if target < 0 {
			target = 0
		}
		if target > upper {
			target = upper
		}
		if target != val {
			vadj.SetValue(target)
		}
		settingsScrollY = vadj.Value()
		return false
	}
	glib.IdleAdd(try)
	glib.TimeoutAdd(16, try)
	glib.TimeoutAdd(50, try)
}

func groupSection(id string) string {
	switch id {
	case "monitor", "style":
		return "Общее"
	case "category", "bg_mode", "bg_alpha", "text_alpha":
		return "HUD"
	case "major":
		return "Мажорные тосты"
	default:
		return "Минорные тосты"
	}
}

func formatGroupValue(g settingsGroup) string {
	switch g.Kind {
	case "choice":
		for _, o := range g.Options {
			if o.ID == g.Active {
				return o.Label
			}
		}
		if len(g.Options) > 0 {
			return g.Options[0].Label
		}
		return "—"
	case "scale":
		if g.Suffix == "%" {
			return fmt.Sprintf("%.0f%s", g.Value, g.Suffix)
		}
		return fmt.Sprintf("%.0f %s", g.Value, g.Suffix)
	default:
		return "—"
	}
}

func settingsSection(text string) *gtk.Label {
	lbl := gtk.NewLabel(text)
	lbl.AddCSSClass("hud-settings-section")
	lbl.SetHAlign(gtk.AlignEnd)
	lbl.SetXAlign(1)
	return lbl
}

func settingsHint(text string) *gtk.Label {
	lbl := gtk.NewLabel(text)
	lbl.AddCSSClass("hint")
	lbl.AddCSSClass("hud-settings-hint")
	lbl.SetHAlign(gtk.AlignEnd)
	lbl.SetXAlign(1)
	return lbl
}

func settingsSep() *gtk.Box {
	sep := gtk.NewBox(gtk.OrientationHorizontal, 0)
	sep.AddCSSClass("hud-settings-sep")
	sep.SetHAlign(gtk.AlignFill)
	sep.SetHExpand(true)
	sep.SetSizeRequest(-1, 1)
	return sep
}

func settingsMaxHeight(monitorIndex int) int {
	mons := shell.ListMonitors(gdk.DisplayGetDefault())
	if len(mons) == 0 {
		return 540
	}
	idx := monitorIndex % len(mons)
	if idx < 0 {
		idx = 0
	}
	h := mons[idx].Geometry().Height()
	if h <= 0 {
		h = 1080
	}
	half := h / 2
	if half < 240 {
		return 240
	}
	return half
}
