//go:build quests_overlay

package notices

import (
	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
	"github.com/quests-inc/quests/go/internal/overlay/stylepack"
)

var minorChange = map[string]string{
	"step_completed": "Шаг выполнен",
	"status_changed": "Статус изменён",
	"quest_deleted":  "Удалено",
	"step_progress":  "Прогресс",
	"pin_changed":    "Закрепление",
	"quest_updated":  "Обновлено",
}

// MinorHost is the bottom-right ephemeral toast.
type MinorHost struct {
	win   *gtk.ApplicationWindow
	slot  *gtk.Box
	queue []Event
	busy  bool
}

// NewMinorHost creates the layer-shell surface.
func NewMinorHost(app *gtk.Application) *MinorHost {
	win := gtk.NewApplicationWindow(app)
	win.SetTitle("Quests Minor")
	win.SetDecorated(false)
	win.SetDefaultSize(320, -1)
	win.AddCSSClass("quests-minor")

	gtk4layershell.InitForWindow(&win.Window)
	gtk4layershell.SetNamespace(&win.Window, shell.NamespaceMinor)
	// Overlay (not Top): same stack as major, above fullscreen apps on that output.
	gtk4layershell.SetLayer(&win.Window, gtk4layershell.LayerShellLayerOverlay)
	gtk4layershell.SetAnchor(&win.Window, gtk4layershell.LayerShellEdgeBottom, true)
	gtk4layershell.SetAnchor(&win.Window, gtk4layershell.LayerShellEdgeRight, true)
	gtk4layershell.SetMargin(&win.Window, gtk4layershell.LayerShellEdgeBottom, 24)
	gtk4layershell.SetMargin(&win.Window, gtk4layershell.LayerShellEdgeRight, 24)
	gtk4layershell.SetKeyboardMode(&win.Window, gtk4layershell.LayerShellKeyboardModeNone)
	gtk4layershell.SetExclusiveZone(&win.Window, 0)
	shell.DisableKeyboardFocus(&win.Window)

	slot := gtk.NewBox(gtk.OrientationVertical, 0)
	win.SetChild(slot)
	win.ConnectRealize(func() {
		shell.ApplyClickThrough(&win.Window)
	})
	return &MinorHost{win: win, slot: slot}
}

// Enqueue queues a minor toast.
func (h *MinorHost) Enqueue(ev Event) {
	h.queue = append(h.queue, ev)
	h.pump()
}

func (h *MinorHost) pump() {
	if h.busy || len(h.queue) == 0 {
		return
	}
	h.busy = true
	ev := h.queue[0]
	h.queue = h.queue[1:]
	h.show(ev)
}

func (h *MinorHost) show(ev Event) {
	clearBox(h.slot)
	card := gtk.NewBox(gtk.OrientationVertical, 2)
	card.AddCSSClass("minor")
	card.SetOpacity(0)

	title := gtk.NewLabel(ev.Title)
	if title.Label() == "" {
		title.SetLabel("—")
	}
	title.AddCSSClass("minor__title")
	title.SetXAlign(0)
	title.SetWrap(true)

	change := minorChange[ev.Kind]
	if change == "" {
		change = ev.Kind
	}
	chg := gtk.NewLabel(change)
	chg.AddCSSClass("minor__change")
	chg.SetXAlign(0)

	card.Append(title)
	card.Append(chg)
	if ev.Detail != "" {
		det := gtk.NewLabel(ev.Detail)
		det.AddCSSClass("minor__detail")
		det.SetXAlign(0)
		det.SetWrap(true)
		card.Append(det)
	}

	h.slot.Append(card)
	h.win.Present()
	shell.ApplyClickThrough(&h.win.Window)

	pack := stylepack.Active()
	animateOpacity(&card.Widget, 0, 1, pack.MinorFadeInMS, func() {
		glib.TimeoutAdd(uint(pack.MinorHoldMS), func() bool {
			animateOpacity(&card.Widget, 1, 0, pack.MinorFadeOutMS, func() {
				clearBox(h.slot)
				h.busy = false
				if len(h.queue) == 0 {
					h.win.Hide()
				} else {
					h.pump()
				}
			})
			return false
		})
	})
}
