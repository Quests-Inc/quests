package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEntryCommitsHistoricalDirectoryAndOddNames(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, ".git")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(gitEnv(), "GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=t@example.test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s %v", args, out, err)
		}
	}
	run("init", "--initial-branch=main")
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unchanged.txt", "folder/child.txt", "\nodd\tname.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("first"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("commit", "-m", "First")
	raw, err := Git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	old := string(raw[:len(raw)-1])
	if err := os.WriteFile(filepath.Join(root, "folder/child.txt"), []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "Change child")
	entries := []Entry{{Path: "folder"}, {Path: "unchanged.txt"}, {Path: "\nodd\tname.txt"}}
	if !localEntryCommits(ctx, dir, "HEAD", "", entries) {
		t.Fatal("missing history")
	}
	if entries[0].LastCommit.Title != "Change child" || entries[1].LastCommit.Title != "First" || entries[2].LastCommit.Title != "First" {
		t.Fatal(entries)
	}
	nested := []Entry{{Path: "folder/child.txt"}}
	if !localEntryCommits(ctx, dir, old, "folder", nested) || nested[0].LastCommit.Title != "First" {
		t.Fatal("historical nested commit", nested)
	}
}
