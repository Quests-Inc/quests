package refs

import "testing"

func TestParseDedupesAndIgnoresJunk(t *testing.T) {
	got := Parse("see note=12 and quest=23; note=12 again, note=12a, attachment=4")
	if len(got) != 3 {
		t.Fatalf("len=%d got=%v", len(got), got)
	}
	want := []Ref{{"note", 12}, {"quest", 23}, {"attachment", 4}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestParseDoesNotPrefixMatch(t *testing.T) {
	got := Parse("note=12 vs note=120")
	if len(got) != 2 || got[0].ID != 12 || got[1].ID != 120 {
		t.Fatalf("got=%v", got)
	}
}

func TestParseKnowsTemplates(t *testing.T) {
	got := Parse("шаблон template=5 и квест quest=7, а mytemplate=9 не ссылка")
	if len(got) != 2 || got[0] != (Ref{Kind: "template", ID: 5}) || got[1] != (Ref{Kind: "quest", ID: 7}) {
		t.Fatalf("got %+v, want template=5 and quest=7 only", got)
	}
}

func TestRepositoryReference(t *testing.T) {
	got := Parse("repo=8 repo=8 myrepo=9 repo=10x")
	if len(got) != 1 || got[0] != (Ref{Kind: "repo", ID: 8}) {
		t.Fatal(got)
	}
}
