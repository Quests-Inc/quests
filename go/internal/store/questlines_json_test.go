package store

import (
	"encoding/json"
	"testing"
	"time"
)

// The web client reads these keys by name: the shape of a questline on the wire is part of the API.
func TestQuestlineReadJSONShape(t *testing.T) {
	icon := "icon.png"
	cases := map[string]struct {
		q    Questline
		keys []string
		null []string
	}{
		"plain": {
			q:    Questline{ID: 3, Title: "T", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)},
			keys: []string{"id", "title", "description", "category_id", "color", "icon", "custom_icon", "created_at", "updated_at", "category_slug", "category_label", "category_color", "icon_url"},
			null: []string{"category_id", "custom_icon", "category_slug", "category_label", "category_color", "icon_url"},
		},
		"custom icon": {
			q:    Questline{ID: 3, Title: "T", CustomIcon: &icon, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)},
			keys: []string{"id", "title", "description", "category_id", "color", "icon", "custom_icon", "created_at", "updated_at", "category_slug", "category_label", "category_color", "icon_url"},
			null: []string{"category_id", "category_slug", "category_label", "category_color"},
		},
	}
	for name, c := range cases {
		raw, err := json.Marshal(questlineToRead(c.q))
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != len(c.keys) {
			t.Errorf("%s: %d keys, want %d: %s", name, len(got), len(c.keys), raw)
		}
		for _, k := range c.keys {
			if _, ok := got[k]; !ok {
				t.Errorf("%s: key %q missing: %s", name, k, raw)
			}
		}
		for _, k := range c.null {
			if got[k] != nil {
				t.Errorf("%s: %q = %v, want null", name, k, got[k])
			}
		}
		if _, ok := got["refs"]; ok {
			t.Errorf("%s: refs must be absent until a single fetch fills it", name)
		}
	}
}
