//go:build quests_overlay

package shell

import (
	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// NudgeStep is the keyboard nudge distance in layout pixels.
const NudgeStep = 24

// NudgeHUD moves a TOP+RIGHT anchored HUD by (dx, dy); positive dx → right, dy → down.
// Returns new margins and whether they changed.
func NudgeHUD(win *gtk.Window, mon *gdk.Monitor, dx, dy int) (marginTop, marginRight int, changed bool) {
	if dx == 0 && dy == 0 {
		return gtk4layershell.GetMargin(win, gtk4layershell.LayerShellEdgeTop),
			gtk4layershell.GetMargin(win, gtk4layershell.LayerShellEdgeRight), false
	}
	winW, winH := windowSize(win)
	maxMT, maxMR := 0, 0
	if mon != nil {
		g := mon.Geometry()
		maxMT = max(0, g.Height()-winH)
		maxMR = max(0, g.Width()-winW)
	}
	mt := gtk4layershell.GetMargin(win, gtk4layershell.LayerShellEdgeTop)
	mr := gtk4layershell.GetMargin(win, gtk4layershell.LayerShellEdgeRight)
	newMT := min(max(0, mt+dy), maxMT)
	newMR := min(max(0, mr-dx), maxMR)
	if newMT == mt && newMR == mr {
		return mt, mr, false
	}
	SetMargins(win, newMT, newMR)
	return newMT, newMR, true
}

// SetMargins applies TOP/RIGHT layer-shell margins (BOTTOM/LEFT zero).
func SetMargins(win *gtk.Window, marginTop, marginRight int) {
	gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeTop, marginTop)
	gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeRight, marginRight)
	gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeBottom, 0)
	gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeLeft, 0)
}

func windowSize(win *gtk.Window) (w, h int) {
	if s := surfaceOf(win); s != nil {
		w, h = s.Width(), s.Height()
	}
	if w <= 0 {
		w = win.Width()
	}
	if h <= 0 {
		h = win.Height()
	}
	return max(1, w), max(1, h)
}
