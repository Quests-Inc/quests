package repository

import (
	"testing"
)

func TestRefAndPathValidation(t *testing.T) {
	for _, s := range []string{"-HEAD", "x..y", "refs/foo:bar", "x\n", "a@{x}", "a b"} {
		if safeRef(s) {
			t.Errorf("accepted ref %q", s)
		}
	}
	for _, s := range []string{"../a", "/etc/passwd", "a/../config", "a\\b"} {
		if safePath(s) {
			t.Errorf("accepted path %q", s)
		}
	}
	if !safeRef("feature/hello") || !safePath("src/main.go") {
		t.Fatal("valid names rejected")
	}
}
func TestRemoteURLBoundaries(t *testing.T) {
	for _, s := range []string{"http://github.com/o/r", "https://github.com.evil/o/r", "https://secret@github.com/o/r", "https://github.com/o/r?token=x", "https://github.com/o/r/tree/main", "https://github.com/o/../r"} {
		if validateRemote("github", s) == nil {
			t.Errorf("accepted %s", s)
		}
	}
	if err := validateRemote("gitlab", "https://gitlab.com/group/sub/project.git"); err != nil {
		t.Fatal(err)
	}
}
