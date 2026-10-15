//go:build quests_overlay

package shell

import (
	"os/exec"
	"strings"

	"github.com/diamondburned/gotk4-layer-shell/pkg/gtk4layershell"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ListMonitors returns connected Gdk monitors.
func ListMonitors(display *gdk.Display) []*gdk.Monitor {
	if display == nil {
		display = gdk.DisplayGetDefault()
	}
	if display == nil {
		return nil
	}
	model := display.Monitors()
	if model == nil {
		return nil
	}
	n := int(model.NItems())
	out := make([]*gdk.Monitor, 0, n)
	for i := 0; i < n; i++ {
		obj := model.Item(uint(i))
		if obj == nil {
			continue
		}
		if mon, ok := obj.Cast().(*gdk.Monitor); ok && mon != nil {
			out = append(out, mon)
		}
	}
	return out
}

// MonitorConnector returns DP-1 / HDMI-A-1 style name.
func MonitorConnector(mon *gdk.Monitor) string {
	if mon == nil {
		return ""
	}
	return strings.TrimSpace(mon.Connector())
}

// ResolveMonitorIndex prefers connector match, else clamps index.
func ResolveMonitorIndex(mons []*gdk.Monitor, connector string, index int) int {
	if len(mons) == 0 {
		return 0
	}
	want := strings.TrimSpace(connector)
	if want != "" {
		for i, mon := range mons {
			if MonitorConnector(mon) == want {
				return i
			}
		}
	}
	if index < 0 {
		return 0
	}
	return index % len(mons)
}

// ApplyMonitor binds the layer surface to a monitor (nil = default).
func ApplyMonitor(win *gtk.Window, mon *gdk.Monitor) {
	gtk4layershell.SetMonitor(win, mon)
}

// FocusNiriOutput asks niri to focus the given connector (for keyboard grab).
func FocusNiriOutput(connector string) bool {
	name := strings.TrimSpace(connector)
	if name == "" {
		return false
	}
	niri, err := exec.LookPath("niri")
	if err != nil {
		return false
	}
	cmd := exec.Command(niri, "msg", "action", "focus-monitor", name)
	return cmd.Run() == nil
}
