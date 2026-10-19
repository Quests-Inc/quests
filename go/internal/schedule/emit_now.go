package schedule

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/store"
)

// A manual emit and a test of a template that makes its quest by a script or a command (a *dynamic* template). Such a quest
// comes from a run on a device, which takes seconds: the request waits for it. What it finds out is one of four things: a
// quest, nothing this time (the script printed null), a failure, or the device did not answer in time.

const (
	manualPeriodPrefix = "manual-"
	testPeriodPrefix   = "test-"
)

// How long a manual emit or a test waits for the device, and how often it looks (variables so a test need not wait a minute).
var (
	emitNowWait = 60 * time.Second
	emitNowPoll = 1500 * time.Millisecond
)

// Outcomes of an emit that was waited for.
const (
	OutcomeQuest   = "quest"
	OutcomeMiss    = "miss"
	OutcomeFailed  = "failed"
	OutcomePending = "pending"
)

// EmitStatusError is an emit that made no quest, with how the API should say so. The HTTP layer reads it through EmitStatus.
type EmitStatusError struct {
	Code    int
	Outcome string
	Detail  string
}

func (e *EmitStatusError) Error() string { return e.Detail }

// EmitStatus: the HTTP status, the outcome and the sentence for the user.
func (e *EmitStatusError) EmitStatus() (int, string, string) { return e.Code, e.Outcome, e.Detail }

// waitEmit drives the emit of one period key until the device has answered or the wait is over. pending says it was over.
func waitEmit(ctx context.Context, st *store.Store, tmpl templateRow, key string, wait time.Duration) (res emitResult, pending bool, err error) {
	deadline := time.Now().Add(wait)
	for {
		res, err = resolveEmit(ctx, st, tmpl, key, timeutil.NowUTC(), poolOpts{ignorePause: true})
		if err != nil {
			return res, false, err
		}
		if res.Spec != nil || res.Err != "" || res.RollID != 0 {
			return res, false, nil
		}
		if time.Now().After(deadline) {
			return res, true, nil
		}
		select {
		case <-ctx.Done():
			return res, true, ctx.Err()
		case <-time.After(emitNowPoll):
		}
	}
}

// emitOutcomeError turns what an emit found into the error a manual emit answers with when it made no quest.
func emitOutcomeError(tmpl templateRow, res emitResult, pending bool) error {
	switch {
	case pending:
		return &EmitStatusError{Code: http.StatusAccepted, Outcome: OutcomePending,
			Detail: "Устройство не ответило за минуту: проверьте, что раннер на связи. Квест не создан."}
	case res.Err != "":
		return &EmitStatusError{Code: http.StatusBadGateway, Outcome: OutcomeFailed, Detail: "Выпуск не удался: " + res.Err}
	}
	return &EmitStatusError{Code: http.StatusOK, Outcome: OutcomeMiss,
		Detail: fmt.Sprintf("Квеста сейчас нет: скрипт или команда шаблона «%s» ничего не вернули.", tmpl.Title)}
}

// PreviewStep is one step of the quest a test emit got.
type PreviewStep struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// EmitPreview is what a test of a template's emit found out; nothing is created.
type EmitPreview struct {
	Outcome     string        `json:"outcome"`
	Title       string        `json:"title,omitempty"`
	Description string        `json:"description,omitempty"`
	Steps       []PreviewStep `json:"steps,omitempty"`
	Ignored     []string      `json:"ignored_fields,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// PreviewEmit runs the template's emit on its device, with its secrets, and says what quest it would make. It creates no
// quest. The attempt shows in the template's emit log under a period of its own (`test-…`).
func PreviewEmit(ctx context.Context, st *store.Store, templateID int64, now time.Time) (EmitPreview, error) {
	tmpl, err := loadTemplateRow(ctx, st, templateID)
	if err != nil {
		return EmitPreview{}, err
	}
	if !emitUsesPool(ctx, st, tmpl) {
		return EmitPreview{}, &EmitStatusError{Code: http.StatusUnprocessableEntity, Outcome: OutcomeFailed,
			Detail: "У шаблона нет скрипта или команды выпуска: нечего проверять."}
	}
	if now.IsZero() {
		now = timeutil.NowUTC()
	}
	res, pending, err := waitEmit(ctx, st, tmpl, fmt.Sprintf("%s%d", testPeriodPrefix, now.UnixNano()), emitNowWait)
	if err != nil {
		return EmitPreview{}, err
	}
	switch {
	case pending:
		return EmitPreview{Outcome: OutcomePending, Error: "устройство не ответило за минуту: проверьте, что раннер на связи"}, nil
	case res.Err != "":
		return EmitPreview{Outcome: OutcomeFailed, Error: res.Err}, nil
	case res.Spec == nil:
		return EmitPreview{Outcome: OutcomeMiss}, nil
	}
	p := EmitPreview{Outcome: OutcomeQuest, Title: res.Spec.Title, Description: res.Spec.Description, Ignored: res.Spec.Ignored}
	if p.Title == "" {
		p.Title = tmpl.Title
	}
	for _, s := range res.Spec.Steps {
		p.Steps = append(p.Steps, PreviewStep{Title: s.Title, Description: s.Description})
	}
	return p, nil
}
