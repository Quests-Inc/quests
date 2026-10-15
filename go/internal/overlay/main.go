//go:build quests_overlay

package overlay

import (
	"fmt"
	"os"

	"github.com/quests-inc/quests/go/internal/overlay/app"
	"github.com/quests-inc/quests/go/internal/overlay/ipc"
)

// Main is the CLI entry: no args → daemon; toggle|monitor|status → IPC.
func Main(args []string) error {
	if len(args) == 0 {
		return app.Run(os.Args)
	}
	cmd := args[0]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Print(usage())
		return nil
	case "toggle", "toggle-input", "monitor", "next-monitor", "cycle-monitor", "status", "refresh":
		reply, err := ipc.Send(cmd)
		if err != nil {
			return err
		}
		fmt.Println(reply)
		return nil
	case "style", "interactive":
		arg := ""
		if len(args) > 1 {
			arg = args[1]
		}
		line := cmd
		if arg != "" {
			line = cmd + " " + arg
		}
		reply, err := ipc.Send(line)
		if err != nil {
			return err
		}
		fmt.Println(reply)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage())
	}
}

func usage() string {
	return `Usage:
  quests-overlay              # start overlay daemon
  quests-overlay toggle       # click-through ↔ interactive (IPC)
  quests-overlay monitor      # cycle output monitor (IPC)
  quests-overlay style [id]   # list / switch style pack
  quests-overlay refresh
  quests-overlay status

Requires: Wayland compositor with wlr-layer-shell (niri, Hyprland, Sway, Plasma).
`
}
