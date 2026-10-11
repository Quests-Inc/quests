package db

import (
	"strings"
	"testing"
)

func TestCheckSchemaVersionMatch(t *testing.T) {
	st, err := checkSchemaVersion(headRevision)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "ok" {
		t.Fatalf("state=%q want ok", st.State)
	}
}

func TestCheckSchemaVersionOlderFails(t *testing.T) {
	// The oldest revision stays older however many migrations are added after it.
	_, err := checkSchemaVersion(revisionOrder[0])
	if err == nil {
		t.Fatal("expected error for older alembic")
	}
	if !strings.Contains(err.Error(), "older than required") {
		t.Fatalf("err=%v", err)
	}
}

func TestCheckSchemaVersionUnknownIsAhead(t *testing.T) {
	st, err := checkSchemaVersion("ffffdeadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "warn" {
		t.Fatalf("state=%q want warn", st.State)
	}
	if !strings.Contains(st.Detail, "новее") {
		t.Fatalf("detail=%q", st.Detail)
	}
}

func TestCheckSchemaVersionKnownAhead(t *testing.T) {
	order := []string{"aaa", "bbb", "ccc"}
	st, err := checkSchemaVersionAgainst("ccc", "bbb", order)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "warn" {
		t.Fatalf("state=%q want warn", st.State)
	}
}

func TestExpectedIsLastInOrder(t *testing.T) {
	if revisionOrder[len(revisionOrder)-1] != headRevision {
		t.Fatalf("headRevision %s must be last in revisionOrder (got %s)",
			headRevision, revisionOrder[len(revisionOrder)-1])
	}
}
