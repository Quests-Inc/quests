//go:build quests_overlay

package notices

import (
	"strings"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/idle"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
	"github.com/quests-inc/quests/go/internal/overlay/stylepack"
)

var majorEyebrow = map[string]string{
	"quest_created":   "Получено задание",
	"quest_appeared":  "Получено задание",
	"quest_started":   "Началось задание",
	"quest_completed": "Задание завершено",
	"quest_failed":    "Задание провалено",
	"quest_expired":   "Задание просрочено",
}

// MajorHost is the top-center overlay toast.
type MajorHost struct {
	win   *gtk.ApplicationWindow
	slot  *gtk.Box
	queue []Event
	busy  bool
}

// NewMajorHost creates the layer-shell surface (hidden until first toast).
func NewMajorHost(app *gtk.Application) *MajorHost {
	win := gtk.NewApplicationWindow(app)
	win.SetTitle("Quests Major")
	win.SetDecorated(false)
	win.AddCSSClass("quests-major")

	gtk4layershell.InitForWindow(&win.Window)
	gtk4layershell.SetNamespace(&win.Window, shell.NamespaceMajor)
	gtk4layershell.SetLayer(&win.Window, gtk4layershell.LayerShellLayerOverlay)
	gtk4layershell.SetAnchor(&win.Window, gtk4layershell.LayerShellEdgeTop, true)
	gtk4layershell.SetMargin(&win.Window, gtk4layershell.LayerShellEdgeTop, 24)
	gtk4layershell.SetKeyboardMode(&win.Window, gtk4layershell.LayerShellKeyboardModeNone)
	// -1 claims the whole output exclusive zone (breaks hot-corners). Stay at 0.
	gtk4layershell.SetExclusiveZone(&win.Window, 0)
	shell.DisableKeyboardFocus(&win.Window)

	outer := gtk.NewBox(gtk.OrientationVertical, 0)
	outer.AddCSSClass("notice-root")
	slot := gtk.NewBox(gtk.OrientationVertical, 0)
	slot.SetHAlign(gtk.AlignCenter)
	outer.Append(slot)
	win.SetChild(outer)

	win.ConnectRealize(func() {
		shell.ApplyClickThrough(&win.Window)
	})

	return &MajorHost{win: win, slot: slot}
}

// Enqueue shows the event after the current toast finishes.
func (h *MajorHost) Enqueue(ev Event) {
	h.queue = append(h.queue, ev)
	h.pump()
}

func (h *MajorHost) pump() {
	if h.busy || len(h.queue) == 0 {
		return
	}
	h.busy = true
	ev := h.queue[0]
	h.queue = h.queue[1:]
	h.show(ev)
}

func (h *MajorHost) show(ev Event) {
	clearBox(h.slot)
	pack := stylepack.Active()

	card := gtk.NewBox(gtk.OrientationVertical, 6)
	card.AddCSSClass("major")
	if sig := strings.ToLower(strings.TrimSpace(ev.Significance)); sig != "" {
		card.AddCSSClass("major--sig-" + sig)
	}
	card.SetOpacity(0)
	card.SetHAlign(gtk.AlignCenter)
	card.SetSizeRequest(420, -1)

	eyebrow := majorEyebrow[ev.Kind]
	if eyebrow == "" {
		eyebrow = ev.Kind
	}
	if pack.ShowSignificance {
		if sig := strings.TrimSpace(ev.Significance); sig != "" {
			eyebrow = eyebrow + " · " + sig
		}
	}
	brow := gtk.NewLabel(eyebrow)
	brow.AddCSSClass("major__eyebrow")
	brow.SetXAlign(0.5)

	title := gtk.NewLabel(ev.Title)
	if title.Label() == "" {
		title.SetLabel("—")
	}
	title.AddCSSClass("major__title")
	title.SetWrap(true)
	title.SetXAlign(0.5)
	title.SetJustify(gtk.JustifyCenter)

	card.Append(brow)
	card.Append(title)
	if pack.ShowMajorBody {
		if d := ev.Description; d != "" {
			body := gtk.NewLabel(d)
			body.AddCSSClass("major__body")
			body.SetWrap(true)
			body.SetXAlign(0.5)
			card.Append(body)
		}
	}

	h.slot.Append(card)
	h.win.Present()
	shell.ApplyClickThrough(&h.win.Window)

	fading := false
	startFade := func() {
		if fading {
			return
		}
		fading = true
		idle.Get().CancelWait()
		animateOpacity(&card.Widget, 1, 0, pack.MajorFadeOutMS, func() {
			clearBox(h.slot)
			h.busy = false
			if len(h.queue) == 0 {
				h.win.Hide()
			} else {
				h.pump()
			}
		})
	}

	animateOpacity(&card.Widget, 0, 1, pack.MajorFadeInMS, func() {
		glib.TimeoutAdd(uint(pack.MajorHoldMS), func() bool {
			// Stay up while AFK; fade when the seat is/becomes active.
			// Fallback: plain dismiss if compositor has no idle-notify.
			if !idle.Get().WaitForActivity(startFade) {
				startFade()
			}
			return false
		})
	})
}

func clearBox(box *gtk.Box) {
	for {
		child := box.FirstChild()
		if child == nil {
			return
		}
		box.Remove(child)
	}
}

func animateOpacity(w *gtk.Widget, start, end float64, durationMS int, done func()) {
	if durationMS <= 0 {
		w.SetOpacity(end)
		if done != nil {
			done()
		}
		return
	}
	frames := durationMS / 30
	if frames < 1 {
		frames = 1
	}
	step := (end - start) / float64(frames)
	v := start
	i := 0
	w.SetOpacity(start)
	glib.TimeoutAdd(30, func() bool {
		i++
		v += step
		if i >= frames {
			w.SetOpacity(end)
			if done != nil {
				done()
			}
			return false
		}
		w.SetOpacity(v)
		return true
	})
}
