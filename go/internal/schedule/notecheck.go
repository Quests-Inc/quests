package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/knowledge"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/store"
)

// A note's check (quest=370) runs through the same machinery as a step's: the server in a trusted
// workspace, or a runner; the note's own secrets in the environment; an interval. Only the reading of
// the answer differs: a bare 0 / 1 on the last line ("is it out of date?") or
// {"outdated": 0|1, "reason": "…"}. Anything else — a failed run, a timeout, an unreadable answer —
// is recorded as an error and leaves the verdict as it was.

// noteJobValidFor: how long a note's check may wait for its device (it is renewed every interval anyway).
const noteJobValidFor = 30 * time.Minute

var errCheckAnswer = errors.New(`нужен ответ 0 или 1 (последняя строка) либо JSON {"outdated": 0|1, "reason": "…"}`)

// ParseNoteCheckOutput reads a check's stdout.
func ParseNoteCheckOutput(out string) (knowledge.NoteCheckResult, error) {
	s := strings.TrimSpace(out)
	if s == "" {
		return knowledge.NoteCheckResult{}, errors.New("пустой вывод: " + errCheckAnswer.Error())
	}
	if strings.HasPrefix(s, "{") {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(s), &raw); err != nil {
			return knowledge.NoteCheckResult{}, fmt.Errorf("битый JSON: %v", err)
		}
		v, ok := raw["outdated"]
		if !ok {
			return knowledge.NoteCheckResult{}, errors.New(`в JSON нет поля "outdated"`)
		}
		if strings.TrimSpace(string(v)) == "null" { // json would read null as 0: not an answer
			return knowledge.NoteCheckResult{}, errors.New(`"outdated" пусто`)
		}
		var res knowledge.NoteCheckResult
		var n float64
		var b bool
		switch {
		case json.Unmarshal(v, &n) == nil && (n == 0 || n == 1):
			res.Outdated = n == 1
		case json.Unmarshal(v, &b) == nil:
			res.Outdated = b
		default:
			return knowledge.NoteCheckResult{}, errors.New(`"outdated" должно быть 0, 1, false или true`)
		}
		if rv, ok := raw["reason"]; ok {
			var reason string
			if json.Unmarshal(rv, &reason) == nil {
				res.Reason = strings.TrimSpace(reason)
			}
		}
		return res, nil
	}
	lines := strings.Split(s, "\n")
	switch strings.TrimSpace(lines[len(lines)-1]) {
	case "1":
		return knowledge.NoteCheckResult{Outdated: true}, nil
	case "0":
		return knowledge.NoteCheckResult{}, nil
	}
	return knowledge.NoteCheckResult{}, fmt.Errorf("%w; получено: %s", errCheckAnswer, firstChars(s, 80))
}

func firstChars(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// noteKey keeps a note's in-flight marker apart from the step ids that share the lock table.
func noteKey(id int64) int64 { return -id }

// TickNotes starts the checks of the notes that are due, in the context's workspace.
func (r *CheckRunner) TickNotes(ctx context.Context) {
	now := timeutil.NowUTC()
	due, err := r.Store.Knowledge().DueNoteChecks(ctx, now)
	if err != nil {
		log.Printf("checks: list note checks: %v", err)
		return
	}
	for _, d := range due {
		r.spawnNote(ctx, d, now)
	}
}

func (r *CheckRunner) spawnNote(ctx context.Context, d knowledge.DueNote, now time.Time) {
	if d.RunOn == nil {
		// Commands run on runners only; one stored without a runner says so instead of staying silent.
		r.noteError(ctx, d.ID, d.Title, store.ErrNoRunner.Error())
		return
	}
	if !r.tryLock(noteKey(d.ID)) {
		return
	}
	if err := r.Store.Knowledge().MarkNoteCheckStarted(ctx, d.ID, now); err != nil {
		r.unlock(noteKey(d.ID))
		log.Printf("checks: mark note %d: %v", d.ID, err)
		return
	}
	r.dispatchNote(ctx, d)
	r.unlock(noteKey(d.ID))
}

func (r *CheckRunner) dispatchNote(ctx context.Context, d knowledge.DueNote) {
	nid := d.ID
	_, _, err := r.Store.EnqueueJob(ctx, store.JobSpec{
		RunnerLabel:    *d.RunOn,
		Kind:           store.JobKindPoll,
		NoteID:         &nid,
		Title:          "Заметка · " + d.Title,
		DedupeKey:      "note:" + strconv.FormatInt(d.ID, 10) + ":poll",
		Command:        d.Command,
		TimeoutSeconds: int(pollTimeout / time.Second),
		ValidFor:       noteJobValidFor,
	})
	if err != nil {
		log.Printf("checks: enqueue note %d on %q: %v", d.ID, *d.RunOn, err)
	}
}

// applyNote turns a finished run into a verdict, or into an error that changes nothing else.
func (r *CheckRunner) applyNote(ctx context.Context, noteID int64, exit int, stdout, stderr string, timedOut bool) {
	n, err := r.Store.Knowledge().GetNote(ctx, noteID)
	if err != nil {
		return // deleted meanwhile
	}
	switch {
	case timedOut:
		r.noteError(ctx, n.ID, n.Title, "таймаут")
		return
	case exit != 0:
		msg := "код " + strconv.Itoa(exit)
		if blob := trimCheckOut(stderr, stdout); blob != "" {
			msg += ": " + blob
		}
		r.noteError(ctx, n.ID, n.Title, msg)
		return
	}
	res, perr := ParseNoteCheckOutput(stdout)
	if perr != nil {
		r.noteError(ctx, n.ID, n.Title, perr.Error())
		return
	}
	changed, err := r.Store.Knowledge().ApplyNoteCheckResult(ctx, n.ID, res, timeutil.NowUTC())
	if err != nil {
		log.Printf("checks: note %d verdict: %v", n.ID, err)
		return
	}
	if !changed {
		return
	}
	detail := "актуальна"
	if res.Outdated {
		detail = "устарела"
		if res.Reason != "" {
			detail += ": " + res.Reason
		}
	}
	r.noteChanged(ctx, n.ID, n.Title, detail)
}

// noteError records a broken run; the log gets a line only when the message is new.
func (r *CheckRunner) noteError(ctx context.Context, id int64, title, msg string) {
	prev, _ := r.Store.Knowledge().GetNoteCheck(ctx, id)
	if err := r.Store.Knowledge().RecordNoteCheckError(ctx, id, msg, timeutil.NowUTC()); err != nil {
		log.Printf("checks: note %d error: %v", id, err)
		return
	}
	if prev.Error != msg {
		r.noteChanged(ctx, id, title, "проверка актуальности: "+msg)
	}
}

func (r *CheckRunner) noteChanged(ctx context.Context, id int64, title, detail string) {
	_ = r.Store.LogEvent(context.WithoutCancel(ctx), store.ChangeEvent{
		Kind: "note_updated", EntityType: "note", EntityID: id, Title: title, Detail: detail,
	})
	if r.Hub == nil {
		return
	}
	silent := ""
	r.Hub.Publish("note_updated", events.PublishOpts{
		Workspace: wsctx.From(ctx), Title: title, Toast: false, Source: "system", Sound: &silent,
		Extra: map[string]any{"note_id": id},
	})
}

// noteJobFinished applies a runner's result to the note that asked for it.
func (r *CheckRunner) noteJobFinished(ctx context.Context, j store.RunnerJob) {
	switch j.State {
	case store.JobSuccess, store.JobFail, store.JobTimeout:
		exit := -1
		if j.ExitCode != nil {
			exit = *j.ExitCode
		}
		r.applyNote(ctx, *j.NoteID, exit, j.Stdout, j.Stderr, j.TimedOut || j.State == store.JobTimeout)
	case store.JobSkipped:
		r.noteJobError(ctx, j, "устройство «"+j.RunnerLabel+"» не вышло на связь")
	case store.JobAbandoned:
		r.noteJobError(ctx, j, "устройство «"+j.RunnerLabel+"» пропало во время проверки, результат неизвестен")
	}
	// canceled / lost: nothing to say — the next interval asks again.
}

func (r *CheckRunner) noteJobError(ctx context.Context, j store.RunnerJob, msg string) {
	if n, err := r.Store.Knowledge().GetNote(ctx, *j.NoteID); err == nil {
		r.noteError(ctx, n.ID, n.Title, msg)
	}
}
