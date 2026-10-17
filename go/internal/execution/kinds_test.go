package execution

import (
	"context"
	"testing"
)

type fakeKind struct {
	name string
	owns func(RunnerJob) bool
	got  []int64
}

func (f *fakeKind) Name() string                            { return f.name }
func (f *fakeKind) Owns(j RunnerJob) bool                   { return f.owns(j) }
func (f *fakeKind) Finished(_ context.Context, j RunnerJob) { f.got = append(f.got, j.ID) }

func TestKindsHandOverToTheFirstOwner(t *testing.T) {
	id := int64(5)
	note := &fakeKind{name: "note", owns: func(j RunnerJob) bool { return j.NoteID != nil }}
	step := &fakeKind{name: "step", owns: func(j RunnerJob) bool { return j.StepID != nil }}
	ks := Kinds{note, step}

	if got := ks.Finished(t.Context(), RunnerJob{ID: 1, StepID: &id}); got != "step" {
		t.Fatalf("a step job went to %q", got)
	}
	if got := ks.Finished(t.Context(), RunnerJob{ID: 2, NoteID: &id, StepID: &id}); got != "note" {
		t.Fatalf("the first owner must win, got %q", got)
	}
	if got := ks.Finished(t.Context(), RunnerJob{ID: 3}); got != "" {
		t.Fatalf("a job nobody owns went to %q", got)
	}
	if len(note.got) != 1 || note.got[0] != 2 || len(step.got) != 1 || step.got[0] != 1 {
		t.Fatalf("note %v step %v", note.got, step.got)
	}
}
