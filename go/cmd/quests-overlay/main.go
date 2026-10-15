//go:build quests_overlay

// Command quests-overlay is the Wayland HUD (GTK4 + layer-shell).
// Build: just build-overlay  (needs -tags quests_overlay, CGO, gtk4-layer-shell).
package main

import (
	"fmt"
	"os"

	"github.com/quests-inc/quests/go/internal/overlay"
)

func main() {
	if err := overlay.Main(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "quests-overlay: %v\n", err)
		os.Exit(1)
	}
}
