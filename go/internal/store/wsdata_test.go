package store

import (
	"strconv"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/refs"
)

func TestRewriteRefsRenumbersAndDefusesStrangers(t *testing.T) {
	ids := map[string]map[int64]int64{"quest": {1: 101, 2: 102}, "note": {7: 207}}
	n := 0
	resolve := func(kind string, id int64) (string, bool) {
		t, ok := kindTable[kind]
		if !ok {
			return "", false
		}
		if nid, ok := ids[t][id]; ok {
			return kind + "=" + itoa(nid), true
		}
		n++
		return kind + "#" + itoa(id), true
	}
	in := "see quest=1, `quest=2`, note=7 and quest=999; id=1 stays, questline=3 too"
	got := refs.Rewrite(in, resolve)
	want := "see quest=101, `quest=102`, note=207 and quest#999; id=1 stays, questline#3 too"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if n != 2 {
		t.Fatalf("dangling = %d, want 2", n)
	}
	if strings.Contains(got, "quest=999") {
		t.Fatal("a stranger id must not stay a live reference")
	}
}

func TestCheckProgressRewrite(t *testing.T) {
	ids := map[int64]int64{5: 55}
	got := checkProgressRe.ReplaceAllStringFunc("quests progress 5 && quests progress 6", func(m string) string {
		num := toInt(checkProgressRe.FindStringSubmatch(m)[1])
		if nid, ok := ids[num]; ok {
			return "quests progress " + itoa(nid)
		}
		return m
	})
	if got != "quests progress 55 && quests progress 6" {
		t.Fatalf("got %q", got)
	}
}

func toInt(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
