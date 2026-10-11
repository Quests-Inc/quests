package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchesIsScopedToTheEventsWorkspace(t *testing.T) {
	ws1 := Hook{Enabled: true, Events: []string{"quest_completed"}, Workspace: 1}
	ws2 := Hook{Enabled: true, Events: []string{"quest_completed"}, Workspace: 2}
	all := Hook{Enabled: true, Events: []string{"quest_completed"}, Workspace: 0}
	cases := []struct {
		name string
		h    Hook
		ws   int64
		want bool
	}{
		{"own workspace", ws2, 2, true},
		{"foreign workspace", ws2, 1, false},
		{"default workspace hook ignores another's events", ws1, 2, false},
		{"operator-wide hook sees every workspace", all, 7, true},
		{"unstamped event (system, or no workspaces) counts as workspace 1", ws1, 0, true},
		{"unstamped event does not reach another workspace's hook", ws2, 0, false},
	}
	for _, c := range cases {
		if got := c.h.Matches("quest_completed", nil, c.ws); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLoadDefaultsMissingWorkspaceToOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	raw := `{"hooks":[
	  {"id":"a","enabled":true,"events":["complete"],"type":"script","command":"true"},
	  {"id":"b","enabled":true,"events":["complete"],"type":"script","command":"true","workspace":0},
	  {"id":"c","enabled":true,"events":["complete"],"type":"script","command":"true","workspace":3}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := Load(path)
	if err != nil || len(list) != 3 {
		t.Fatalf("load: %v %d", err, len(list))
	}
	if list[0].Workspace != 1 || list[1].Workspace != 0 || list[2].Workspace != 3 {
		t.Fatalf("workspaces = %d %d %d, want 1 0 3", list[0].Workspace, list[1].Workspace, list[2].Workspace)
	}
	// Saving keeps an explicit 0 ("all") distinguishable from "absent".
	if err := Save(list, path); err != nil {
		t.Fatal(err)
	}
	again, _ := Load(path)
	if again[1].Workspace != 0 || again[0].Workspace != 1 {
		t.Fatalf("round trip lost the scope: %d %d", again[0].Workspace, again[1].Workspace)
	}
}
