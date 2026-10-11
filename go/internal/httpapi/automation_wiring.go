package httpapi

import (
	"context"
	"time"

	"github.com/valirum/quests/go/internal/automation"
	automationhttp "github.com/valirum/quests/go/internal/automation/http"
	"github.com/valirum/quests/go/internal/schedule"
)

// automationHTTP is the automation context's HTTP face, wired to this server (built on first use). What belongs to
// other contexts or to the scheduler comes in as functions.
func (s *Server) automationHTTP() *automationhttp.Handler {
	s.automationOnce.Do(func() {
		s.ah = &automationhttp.Handler{
			Store:        s.Store.Automation(),
			Hub:          s.Hub,
			RequireOwner: s.identityHTTP().RequireOwner,
			ResolveRefs:  s.resolveRefs,
			MaterializeDue: func(ctx context.Context) {
				_, _ = schedule.MaterializeDue(ctx, s.Store, s.Hub, time.Time{}, nil)
			},
			EmitTemplate: func(ctx context.Context, id int64, now time.Time) (int64, error) {
				return schedule.MaterializeTemplateManual(ctx, s.Store, s.Hub, id, now, nil)
			},
			ErrTemplateNotFound: schedule.ErrTemplateNotFound,
			RunnerDone: func(ctx context.Context, j automation.RunnerJob) {
				if s.RunnerDone != nil {
					s.RunnerDone(ctx, j)
				}
			},
		}
	})
	return s.ah
}
