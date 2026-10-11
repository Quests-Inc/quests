package knowledgehttp

import (
	"testing"
)

func TestApplyDescriptionInsert(t *testing.T) {
	src := "αβγ hello"

	got, err := applyDescriptionInsert(src, "!", nil, nil, nil)
	if err != nil || got != "αβγ hello!" {
		t.Fatalf("append: %q %v", got, err)
	}

	after := "β"
	got, err = applyDescriptionInsert(src, "X", &after, nil, nil)
	if err != nil || got != "αβXγ hello" {
		t.Fatalf("after: %q %v", got, err)
	}

	before := " hello"
	got, err = applyDescriptionInsert(src, "|", nil, &before, nil)
	if err != nil || got != "αβγ| hello" {
		t.Fatalf("before: %q %v", got, err)
	}

	at := 3
	got, err = applyDescriptionInsert(src, "-", nil, nil, &at)
	if err != nil || got != "αβγ- hello" {
		t.Fatalf("at: %q %v", got, err)
	}

	miss := "нет"
	if _, err := applyDescriptionInsert(src, "x", &miss, nil, nil); err == nil {
		t.Fatal("missing after should error")
	}

	bad := -1
	if _, err := applyDescriptionInsert(src, "x", nil, nil, &bad); err == nil {
		t.Fatal("bad at should error")
	}

	a, b := "a", "b"
	if _, err := applyDescriptionInsert(src, "x", &a, &b, nil); err == nil {
		t.Fatal("two anchors should error")
	}
}

func TestApplyDescriptionReplace(t *testing.T) {
	src := "one two one"

	got, err := applyDescriptionReplace(src, "one", "ONE", false)
	if err != nil || got != "ONE two one" {
		t.Fatalf("first: %q %v", got, err)
	}

	got, err = applyDescriptionReplace(src, "one", "ONE", true)
	if err != nil || got != "ONE two ONE" {
		t.Fatalf("all: %q %v", got, err)
	}

	if _, err := applyDescriptionReplace(src, "нет", "x", false); err == nil {
		t.Fatal("missing find should error")
	}
	if _, err := applyDescriptionReplace(src, "", "x", false); err == nil {
		t.Fatal("empty find should error")
	}
}
