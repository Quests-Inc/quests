//go:build quests_overlay

package shell

import (
	"sync"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// HUD is keyboard-only: pointer always click-through. Interactive mode only
// grabs the seat keyboard (Exclusive).
var (
	inputRegionMu sync.Mutex
	inputRegions  = map[*gdk.Surface]*cairo.Region{}
)

// SurfaceOf returns the Gdk surface for a window (nil if not realized).
func SurfaceOf(win *gtk.Window) *gdk.Surface {
	return surfaceOf(win)
}

func surfaceOf(win *gtk.Window) *gdk.Surface {
	s := win.Surface()
	if s == nil {
		return nil
	}
	if concrete, ok := s.(*gdk.Surface); ok {
		return concrete
	}
	return gdk.BaseSurface(s)
}

// pinEmptyRegion reuses one empty region per surface: this runs several times
// a second, and allocating a fresh native region each call leaked memory.
func pinEmptyRegion(surface *gdk.Surface) {
	inputRegionMu.Lock()
	empty := inputRegions[surface]
	if empty == nil {
		var err error
		empty, err = cairo.RegionCreate()
		if err != nil || empty == nil {
			inputRegionMu.Unlock()
			return
		}
		inputRegions[surface] = empty
	}
	inputRegionMu.Unlock()
	surface.SetInputRegion(empty)
}

// DisableKeyboardFocus drops Exclusive grab.
func DisableKeyboardFocus(win *gtk.Window) {
	win.SetCanFocus(false)
	win.SetFocusable(false)
	win.SetFocusOnClick(false)
	win.SetCanTarget(false)
	gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeNone)
}

// ForceClickThrough applies empty input region + exclusive_zone 0 + no keyboard.
// Call on realize/map and after every child rebuild.
func ForceClickThrough(win *gtk.Window) {
	gtk4layershell.SetExclusiveZone(win, 0)
	DisableKeyboardFocus(win)
	if surface := surfaceOf(win); surface != nil {
		pinEmptyRegion(surface)
	}
}

// ApplyInputMode always keeps an empty pointer region (click-through) and
// exclusive_zone=0 (so niri hot-corners keep working).
// interactive=true → Exclusive keyboard grab only.
func ApplyInputMode(win *gtk.Window, interactive bool) bool {
	surface := surfaceOf(win)
	if surface == nil {
		return false
	}
	gtk4layershell.SetExclusiveZone(win, 0)
	pinEmptyRegion(surface)
	win.SetCanTarget(false)
	win.SetFocusOnClick(false)

	if interactive {
		win.SetCanFocus(true)
		win.SetFocusable(true)
		gtk4layershell.SetKeyboardMode(win, gtk4layershell.LayerShellKeyboardModeExclusive)
		return true
	}
	DisableKeyboardFocus(win)
	pinEmptyRegion(surface)
	return true
}

// ApplyClickThrough is for notice surfaces that never take pointer/keyboard.
func ApplyClickThrough(win *gtk.Window) {
	ForceClickThrough(win)
}
