//go:build quests_overlay

// Package app wires the GTK application and owns the main loop.
package app

import (
	"fmt"
	"os"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/hud"
	"github.com/quests-inc/quests/go/internal/overlay/idle"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
)

const appID = "dev.quests.overlay"

// Run starts the GTK application (blocks until quit).
func Run(osArgs []string) error {
	// gdk/gtk must be up before layer-shell probes the Wayland globals.
	gtk.Init()
	if !shell.Supported() {
		return fmt.Errorf("wlr-layer-shell is not available (need niri/Hyprland/Sway/Plasma Wayland)")
	}

	gtkApp := gtk.NewApplication(appID+os.Getenv("QUESTS_DBG_APPSUFFIX"), gio.ApplicationFlagsNone)
	gtkApp.ConnectActivate(func() {
		_ = idle.Get().Start()
		hud.Activate(gtkApp)
	})
	// gio.Application "shutdown" (via embedded base).
	gtkApp.Application.ConnectShutdown(func() {
		idle.Get().Stop()
	})

	code := gtkApp.Run(osArgs)
	if code != 0 {
		os.Exit(code)
	}
	return nil
}
