package schedule

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestWorkspacePurgeRemovesOnlyExpiredRepositoryDirectory(t *testing.T) {
	st := openEmitSpecDB(t)
	user, err := st.Identity.CreateUser("repo-purge", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := st.Identity.CreateWorkspace(bg(), user.ID, "Expired sources")
	if err != nil {
		t.Fatal(err)
	}
	active, err := st.Identity.CreateWorkspace(bg(), user.ID, "Active sources")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.DB.ExecContext(bg(), `UPDATE workspace SET deleted_at=now()-interval '31 days' WHERE id=?`, expired.ID); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	expiredDir := filepath.Join(root, "repositories", strconv.FormatInt(expired.ID, 10))
	activeDir := filepath.Join(root, "repositories", strconv.FormatInt(active.ID, 10))
	for _, dir := range []string{expiredDir, activeDir} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("source"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	PurgeDueWorkspaces(bg(), st, nil, root)
	if _, err = os.Stat(expiredDir); !os.IsNotExist(err) {
		t.Fatal("expired sources retained")
	}
	if _, err = os.Stat(activeDir); err != nil {
		t.Fatal("active sources removed")
	}
}
