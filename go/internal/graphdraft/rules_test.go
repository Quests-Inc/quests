package graphdraft

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/store"
)

func TestEditToken(t *testing.T) {
	if got := editToken("see note=5 and note=55 here", "note=5", false); !strings.Contains(got, "note=55") || strings.Contains(got, "note=5 ") {
		t.Fatalf("remove touched the wrong token: %q", got)
	}
	if got := editToken("text", "note=7", true); got != "text\n\nnote=7" {
		t.Fatalf("append = %q", got)
	}
	if got := editToken("text\n\nnote=7", "note=7", true); got != "text\n\nnote=7" {
		t.Fatalf("append must be idempotent: %q", got)
	}
}

func TestDraftCascadeRules(t *testing.T) {
	mk := func(id int64, kind, payload string) store.DraftOp {
		return store.DraftOp{ID: id, Kind: kind, Payload: json.RawMessage(payload)}
	}
	ids := func(in []int64) string { return fmt.Sprint(in) }

	// A made thing takes what uses it, transitively; unrelated ops stay.
	ops := []store.DraftOp{
		mk(1, "create_quest", `{"fields":{"title":"A"}}`),
		mk(2, "create_step", `{"fields":{"title":"s"},"quest":"quest:t1"}`),
		mk(3, "add_ref", `{"from":"step:t2","to":"note:5"}`),
		mk(4, "add_ref", `{"from":"quest:9","to":"note:5"}`),
		mk(12, "create_note", `{"fields":{"title":"N"}}`),
		mk(13, "add_ref", `{"from":"quest:9","to":"note:t12"}`),
	}
	if got := ids(dependents(ops, 1)); got != "[1 2 3]" {
		t.Fatalf("cascade from op 1 = %s, want [1 2 3]", got)
	}
	// Op 1 must not be confused with op 12 (t1 vs t12).
	if got := ids(dependents(ops, 12)); got != "[12 13]" {
		t.Fatalf("cascade from op 12 = %s, want [12 13]", got)
	}
	// Free text that looks like a placeholder is not a reference.
	text := []store.DraftOp{
		mk(1, "create_quest", `{"fields":{"title":"A"}}`),
		mk(2, "create_note", `{"fields":{"title":"n","description":"see quest:t1"}}`),
	}
	if got := ids(dependents(text, 1)); got != "[1]" {
		t.Fatalf("text must not count as a reference: %s", got)
	}

	// Add / remove of the same link: dropping the first takes the next
	// opposite op with it, but not what follows.
	pair := []store.DraftOp{
		mk(1, "add_ref", `{"from":"quest:1","to":"note:2"}`),
		mk(2, "remove_ref", `{"from":"quest:1","to":"note:2"}`),
		mk(3, "add_ref", `{"from":"quest:1","to":"note:2"}`),
		mk(4, "add_ref", `{"from":"quest:1","to":"note:3"}`),
	}
	if got := ids(dependents(pair, 1)); got != "[1 2]" {
		t.Fatalf("dropping an add must drop the next remove only: %s", got)
	}
	if got := ids(dependents(pair, 3)); got != "[3]" {
		t.Fatalf("the last op of a pair depends on nothing after it: %s", got)
	}
	if got := ids(dependents(pair, 4)); got != "[4]" {
		t.Fatalf("an unrelated link stays alone: %s", got)
	}
}
