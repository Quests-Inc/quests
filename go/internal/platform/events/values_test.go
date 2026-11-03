package events

import "testing"

func TestValuesChangedKeepsEventEnvelope(t *testing.T) {
	for _, typ := range []string{"int", "float", "str", "bool", ""} {
		h := New()
		h.ValuesChanged(7, ValueChange{OwnerKind: "note", OwnerID: 342, Name: "protein_g", Type: typ, Value: 3})
		p := h.recent[0]
		if p["type"] != "quests_changed" || p["kind"] != "values_changed" || p["workspace"] != int64(7) {
			t.Fatalf("broken envelope: %+v", p)
		}
		if typ != "" && p["value_type"] != typ {
			t.Fatalf("missing value type: %+v", p)
		}
		if typ == "str" && p["value"] != nil {
			t.Fatalf("text leaked: %+v", p)
		}
	}
}
