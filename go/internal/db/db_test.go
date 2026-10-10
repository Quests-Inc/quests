package db

import (
	"strings"
	"testing"
)

func TestCheckAlembicVersionMatch(t *testing.T) {
	st, err := checkAlembicVersion(expectedAlembic)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "ok" {
		t.Fatalf("state=%q want ok", st.State)
	}
}

func TestCheckAlembicVersionOlderFails(t *testing.T) {
	// The oldest revision stays older however many migrations are added after it.
	_, err := checkAlembicVersion(alembicOrder[0])
	if err == nil {
		t.Fatal("expected error for older alembic")
	}
	if !strings.Contains(err.Error(), "older than required") {
		t.Fatalf("err=%v", err)
	}
}

func TestCheckAlembicVersionUnknownIsAhead(t *testing.T) {
	st, err := checkAlembicVersion("ffffdeadbeef")
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

func TestCheckAlembicVersionKnownAhead(t *testing.T) {
	order := []string{"aaa", "bbb", "ccc"}
	st, err := checkAlembicVersionAgainst("ccc", "bbb", order)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "warn" {
		t.Fatalf("state=%q want warn", st.State)
	}
}

func TestExpectedIsLastInOrder(t *testing.T) {
	if alembicOrder[len(alembicOrder)-1] != expectedAlembic {
		t.Fatalf("expectedAlembic %s must be last in alembicOrder (got %s)",
			expectedAlembic, alembicOrder[len(alembicOrder)-1])
	}
}
