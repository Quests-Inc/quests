package httpapi

import (
	"context"
	"time"

	automationhttp "github.com/quests-inc/quests/go/internal/automation/http"
	"github.com/quests-inc/quests/go/internal/execution"
	executionhttp "github.com/quests-inc/quests/go/internal/execution/http"
	"github.com/quests-inc/quests/go/internal/schedule"
)

// automationHTTP is the automation context's HTTP face, wired to this server (built on first use). What belongs to
// other contexts or to the scheduler comes in as functions.
func (s *Server) automationHTTP() *automationhttp.Handler {
	s.automationOnce.Do(func() {
		s.ah = &automationhttp.Handler{
			Store:        s.Store.Automation,
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
		}
	})
	return s.ah
}

// executionHTTP is the execution context's HTTP face: runners, their protocol and the secrets of commands.
func (s *Server) executionHTTP() *executionhttp.Handler {
	s.executionOnce.Do(func() {
		s.eh = &executionhttp.Handler{
			Store:        s.Store.Execution,
			RequireOwner: s.identityHTTP().RequireOwner,
			SubjectExists: func(ctx context.Context, kind string, id int64) (bool, error) {
				switch kind {
				case execution.SubjectStep:
					_, err := s.Store.Journal.QuestIDForStep(ctx, id)
					return err == nil, nil
				case execution.SubjectNote:
					_, err := s.Store.Knowledge.GetNote(ctx, id)
					return err == nil, nil
				}
				return false, nil
			},
			ClearCommand: func(ctx context.Context, kind string, id int64) error {
				if kind == execution.SubjectStep {
					return s.Store.Journal.ClearStepCommand(ctx, id)
				}
				return s.Store.Knowledge.ClearNoteCommand(ctx, id)
			},
			RunnerDone: func(ctx context.Context, j execution.RunnerJob) {
				if s.RunnerDone != nil {
					s.RunnerDone(ctx, j)
				}
			},
		}
	})
	return s.eh
}
