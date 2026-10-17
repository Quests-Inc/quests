package executionhttp

import "github.com/quests-inc/quests/go/internal/execution"

// RunnerView is a runner as the settings page shows it.
type RunnerView struct {
	execution.Runner `tstype:",extends"`
	Online           bool `json:"online"`
	// Queued / Running count the runner's unfinished jobs.
	Queued  int `json:"queued"`
	Running int `json:"running"`
}
