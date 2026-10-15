//go:build quests_overlay

// Package notices routes API events to major/minor toast surfaces.
package notices

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
)

var majorKinds = map[string]struct{}{
	"quest_created":   {},
	"quest_appeared":  {},
	"quest_started":   {},
	"quest_completed": {},
	"quest_failed":    {},
	"quest_expired":   {},
}

// Event is a HUD-facing slice of /api/events items.
type Event struct {
	Kind         string
	Title        string
	Description  string
	Detail       string
	Significance string
	Automated    bool
	Toast        bool
}

// Router splits events into major / minor lanes.
type Router struct {
	MajorEnabled bool
	MinorMode    string // off | toast | log (log → toast for now)
	major        *MajorHost
	minor        *MinorHost
}

// NewRouter builds major/minor hosts.
func NewRouter(app *gtk.Application, majorEnabled bool, minorMode string) *Router {
	mode := strings.ToLower(strings.TrimSpace(minorMode))
	if mode != "off" && mode != "toast" && mode != "log" {
		mode = "toast"
	}
	return &Router{
		MajorEnabled: majorEnabled,
		MinorMode:    mode,
		major:        NewMajorHost(app),
		minor:        NewMinorHost(app),
	}
}

// SetMajorEnabled toggles major toast lane.
func (r *Router) SetMajorEnabled(on bool) {
	r.MajorEnabled = on
}

// SetMinorMode sets off|toast|log.
func (r *Router) SetMinorMode(mode string) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "off" && mode != "toast" && mode != "log" {
		mode = "toast"
	}
	r.MinorMode = mode
}

// SetMonitor pins major/minor toast surfaces to the same output as the HUD.
func (r *Router) SetMonitor(mon *gdk.Monitor) {
	if r.major != nil && r.major.win != nil {
		shell.ApplyMonitor(&r.major.win.Window, mon)
	}
	if r.minor != nil && r.minor.win != nil {
		shell.ApplyMonitor(&r.minor.win.Window, mon)
	}
}

// Enqueue routes one event.
func (r *Router) Enqueue(ev Event) {
	if !ev.Toast {
		return
	}
	kind := ev.Kind
	if _, isMajor := majorKinds[kind]; isMajor {
		demote := ev.Automated && (kind == "quest_created" || kind == "quest_appeared" || kind == "quest_started")
		if !demote {
			if r.MajorEnabled {
				r.major.Enqueue(ev)
			}
			return
		}
	}
	if r.MinorMode == "toast" || r.MinorMode == "log" {
		r.minor.Enqueue(ev)
	}
}
