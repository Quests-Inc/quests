package limits

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistencePrecedenceValidationReset(t *testing.T) {
	old := Path()
	t.Cleanup(func() { mu.Lock(); filename = old; mu.Unlock() })
	Configure(t.TempDir())
	n := int64(2048)
	if err := Change("upload_bytes", &n); err != nil {
		t.Fatal(err)
	}
	if Get("upload_bytes") != n {
		t.Fatal("saved value missing")
	}
	t.Setenv("QUESTS_MAX_UPLOAD_MB", "3")
	rows, err := List()
	if err != nil || rows["upload_bytes"].Value != 3<<20 || rows["upload_bytes"].Source != "env" {
		t.Fatal(rows, err)
	}
	t.Setenv("QUESTS_MAX_UPLOAD_MB", "")
	Configure(filepath.Dir(Path()))
	if Get("upload_bytes") != n {
		t.Fatal("restart lost value")
	}
	bad := int64(-1)
	if Change("upload_bytes", &bad) == nil || Change("not_a_limit", &n) == nil {
		t.Fatal("invalid accepted")
	}
	if err := Change("upload_bytes", nil); err != nil {
		t.Fatal(err)
	}
	if Get("upload_bytes") != Specs["upload_bytes"].Default {
		t.Fatal("reset failed")
	}
	if _, err := os.Stat(Path() + ".audit.jsonl"); err != nil {
		t.Fatal(err)
	}
}

func TestRestartBudgetsReportAppliedValues(t *testing.T) {
	old := Path()
	mu.RLock()
	oldStartup := startup
	mu.RUnlock()
	t.Cleanup(func() { mu.Lock(); filename = old; startup = oldStartup; mu.Unlock() })
	Configure(t.TempDir())
	Freeze()
	n := int64(9)
	if err := Change("egress_concurrency", &n); err != nil {
		t.Fatal(err)
	}
	if Applied("egress_concurrency") != 4 || Get("egress_concurrency") != 9 {
		t.Fatal("pending restart not distinguished")
	}
	Freeze()
	if Applied("egress_concurrency") != 9 {
		t.Fatal("restart did not apply")
	}
}
