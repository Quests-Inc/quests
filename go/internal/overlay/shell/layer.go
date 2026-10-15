//go:build quests_overlay

// Package shell wraps gtk4-layer-shell for HUD / notice surfaces.
package shell

import (
	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	NamespaceHUD   = "quests-overlay"
	NamespaceMajor = "quests-major"
	NamespaceMinor = "quests-minor"
)

// Supported reports whether the compositor speaks zwlr_layer_shell_v1.
func Supported() bool {
	return gtk4layershell.IsSupported()
}

// PrepareHUD turns win into a top-right layer-shell surface (HUD).
// Call before the window is realized / presented.
func PrepareHUD(win *gtk.Window, marginTop, marginRight int) {
	if marginTop < 0 {
		marginTop = 24
	}
	if marginRight < 0 {
		marginRight = 24
	}
	gtk4layershell.InitForWindow(win)
	gtk4layershell.SetNamespace(win, NamespaceHUD)
	gtk4layershell.SetLayer(win, gtk4layershell.LayerShellLayerTop)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeRight, true)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeLeft, false)
	gtk4layershell.SetAnchor(win, gtk4layershell.LayerShellEdgeBottom, false)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeNone)
	// Never auto-reserve the top strip — that kills niri hot-corners and feels fullscreen.
	gtk4layershell.SetExclusiveZone(win, 0)
	gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeTop, marginTop)
	gtk4layershell.SetMargin(win, gtk4layershell.LayerShellEdgeRight, marginRight)
	win.SetCanFocus(false)
	win.SetFocusable(false)
	win.SetFocusOnClick(false)
	win.SetCanTarget(false)
}
