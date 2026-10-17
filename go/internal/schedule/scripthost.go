package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// scriptHost is what the evaluator needs from the rest of the system for one run: what the source nodes read and what the
// sink nodes do. It is the one place where the node catalog meets the contexts.
type scriptHost struct {
	r    *CheckRunner
	subj runSubject
}

func (r *CheckRunner) hostFor(s runSubject) graph.Host { return scriptHost{r: r, subj: s} }

func (h scriptHost) Source(ctx context.Context, n graph.Node) (map[string]any, error) {
	kind := strings.TrimPrefix(n.Type, "source.")
	if kind != h.subj.Type {
		return nil, fmt.Errorf("источник %q в запуске для %q", kind, h.subj.Type)
	}
	ref := graph.Ref{Kind: kind, ID: h.subj.ID}
	switch kind {
	case "note":
		note, err := h.r.Store.Knowledge.GetNote(ctx, h.subj.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ref": ref, "secrets": graph.Secrets{}, "title": note.Title}, nil
	}
	return nil, fmt.Errorf("источник %q ещё не подключён", n.Type)
}

func (h scriptHost) Sink(ctx context.Context, n graph.Node, in map[string]graph.Token) error {
	switch n.Type {
	case "sink.note_verdict":
		return h.r.sinkNoteVerdict(ctx, in)
	case "sink.note_error":
		return h.r.sinkNoteError(ctx, in)
	}
	return fmt.Errorf("приёмник %q ещё не подключён", n.Type)
}

// beforeResume is the bookkeeping of the schedule that belongs to the engine, not to a graph: it runs when a job of the
// subject has come back, whatever the graph then does with the answer.
func (r *CheckRunner) beforeResume(context.Context, runSubject, graph.CodeOutcome) {}

func refOf(in map[string]graph.Token) (graph.Ref, error) {
	ref, ok := in["ref"].Value.(graph.Ref)
	if !ok {
		return graph.Ref{}, errors.New("нет ссылки на сущность")
	}
	return ref, nil
}

// sinkNoteVerdict turns an `outdated` answer into the verdict of a note (what applyNote did after reading it).
func (r *CheckRunner) sinkNoteVerdict(ctx context.Context, in map[string]graph.Token) error {
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	n, err := r.Store.Knowledge.GetNote(ctx, ref.ID)
	if err != nil {
		return nil // deleted meanwhile
	}
	outdated, _ := in["outdated"].Value.(bool)
	reason, _ := in["reason"].Value.(string)
	res := knowledge.NoteCheckResult{Outdated: outdated, Reason: strings.TrimSpace(reason)}
	changed, err := r.Store.Knowledge.ApplyNoteCheckResult(ctx, n.ID, res, timeutil.NowUTC())
	if err != nil {
		return fmt.Errorf("вердикт заметки %d: %w", n.ID, err)
	}
	if !changed {
		return nil
	}
	detail := "актуальна"
	if res.Outdated {
		detail = "устарела"
		if res.Reason != "" {
			detail += ": " + res.Reason
		}
	}
	r.noteChanged(ctx, n.ID, n.Title, detail)
	return nil
}

// sinkNoteError records a broken check on the note and leaves its verdict alone.
func (r *CheckRunner) sinkNoteError(ctx context.Context, in map[string]graph.Token) error {
	ref, err := refOf(in)
	if err != nil {
		return err
	}
	n, err := r.Store.Knowledge.GetNote(ctx, ref.ID)
	if err != nil {
		return nil
	}
	msg, _ := in["message"].Value.(string)
	r.noteError(ctx, n.ID, n.Title, msg)
	return nil
}
