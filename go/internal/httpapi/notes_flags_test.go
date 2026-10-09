package httpapi

import "testing"

func TestParseNoteFlagFilter(t *testing.T) {
	f := parseNoteFlagFilter("outdated, -private,pin")
	if f.bad != "" || len(f.include) != 2 || f.include[0] != "outdated" || f.include[1] != "pinned" || len(f.exclude) != 1 || f.exclude[0] != "private" {
		t.Fatalf("filter = %+v", f)
	}
	if f := parseNoteFlagFilter("stale,nope"); f.bad != "nope" {
		t.Fatalf("an unknown flag must be reported: %+v", f)
	}
	if parseNoteFlagFilter("").active() {
		t.Fatal("empty flags must not filter")
	}
}
