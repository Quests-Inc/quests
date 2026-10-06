package schedule

import (
	"context"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/store"
)

const PollInterval = 15 * time.Second

func MaintenanceEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("QUESTS_MAINTENANCE")))
	switch raw {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// RunMaintenanceLoop mirrors Python main.maintenance_loop.
func RunMaintenanceLoop(ctx context.Context, st *store.Store, hub *events.Hub, windows *WindowNotifier, checks *CheckRunner) {
	if !MaintenanceEnabled() {
		log.Printf("maintenance disabled (QUESTS_MAINTENANCE=0)")
		return
	}
	if checks != nil {
		checks.FailStaleRunning(ctx)
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	pool := newPoolRunner()
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		runOnce(ctx, st, hub, windows, rng, checks, pool)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, st *store.Store, hub *events.Hub, windows *WindowNotifier, rng *rand.Rand, checks *CheckRunner, pool *poolRunner) {
	if _, err := ExpireOverdue(ctx, st, hub); err != nil {
		log.Printf("expire: %v", err)
	}
	if windows != nil {
		if _, err := windows.Notify(ctx, st, hub); err != nil {
			log.Printf("window_start: %v", err)
		}
	}
	if _, err := materializeDue(ctx, st, hub, time.Time{}, rng, pool); err != nil {
		log.Printf("materialize: %v", err)
	}
	if checks != nil {
		checks.ExpireRunnerJobs(ctx)
		checks.Tick(ctx)
	}
}
