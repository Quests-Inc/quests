package automationhttp

import "github.com/quests-inc/quests/go/internal/automation"

// RunnerView is a runner as the settings page shows it.
type RunnerView struct {
	automation.Runner `tstype:",extends"`
	Online            bool `json:"online"`
	// Queued / Running count the runner's unfinished jobs.
	Queued  int `json:"queued"`
	Running int `json:"running"`
}
