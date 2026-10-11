// Package archtest holds architecture gates that run with the ordinary tests (ADR-0001).
package archtest

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maxLines is the longest a non-test Go file may be: past it the file is doing several jobs and wants splitting by topic.
const maxLines = 600

func TestGoFilesStayShort(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		n := 0
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			n++
		}
		if n > maxLines {
			t.Errorf("%s: %d lines, the limit is %d — split it by topic", path, n, maxLines)
		}
		return sc.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}
