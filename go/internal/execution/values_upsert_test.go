package execution

import (
	"encoding/json"
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/limits"
	"path/filepath"
	"sync"
	"testing"
)

func TestTypedBulkForwardDependenciesAndRollback(t *testing.T) {
	s := openSecretsDB(t)
	ctx := valueCtx()
	id := insertNote(t, s, "bulk", nil, false)
	o := SecretOwner{SecretOwnerNote, id}
	var input []ValueInput
	raw := `[{"name":"mod","type":"int","formula":"(stat-10)/2"},{"name":"stat","type":"int","value":9},{"name":"flag","type":"bool","value":false},{"name":"caption","type":"str","value":""},{"name":"ratio","type":"float","value":0.5}]`
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		input = append(input, ValueInput{Name: fmt.Sprintf("n%d", i), Type: "int", Value: json.RawMessage(`0`)})
	}
	rows, err := s.SetNamedValues(ctx, o, input, false)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Value != "-1" || rows[2].Value != "false" || rows[3].Value != "" || len(rows) != 30 {
		t.Fatal(rows)
	}
	broken := []string{
		`[{"name":"stat","type":"int","value":20},{"name":"x","type":"int","formula":"missing+1"}]`,
		`[{"name":"x","type":"int","formula":"y"},{"name":"y","type":"int","formula":"x"}]`,
		`[{"name":"x","type":"int","value":0,"formula":"1"}]`,
		`[{"name":"x","type":"int","value":1},{"name":"x","type":"int","value":2}]`,
		`[{"name":"x","type":"bool","value":"false"}]`,
		`[{"name":"x","type":"str","value":null}]`,
		`[{"name":"x","type":"int","formula":"1/0"}]`,
	}
	for _, raw := range broken {
		var in []ValueInput
		_ = json.Unmarshal([]byte(raw), &in)
		if _, err := s.SetNamedValues(ctx, o, in, false); err == nil {
			t.Fatalf("accepted %s", raw)
		}
		set, err := s.ResolveNamedValues(ctx, o, false)
		if err != nil || len(set.Own) != 30 {
			t.Fatalf("partial state: %+v %v", set, err)
		}
		for _, v := range set.Own {
			if v.Name == "stat" && v.Value != "9" {
				t.Fatal("partial stat write")
			}
		}
	}
	if _, err := s.SetNamedValues(ctx, o, []ValueInput{{Name: "mod", Type: "int", Value: json.RawMessage(`4`)}}, false); err != nil {
		t.Fatal(err)
	}
}
func TestTypedBulkConcurrentQuota(t *testing.T) {
	s := openSecretsDB(t)
	id := insertNote(t, s, "parallel", nil, false)
	o := SecretOwner{SecretOwnerNote, id}
	var wg sync.WaitGroup
	results := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.SetNamedValues(valueCtx(), o, []ValueInput{{Name: fmt.Sprintf("v%d", i), Type: "int", Value: json.RawMessage(`1`)}}, false)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		}
	}
	set, err := s.ResolveNamedValues(valueCtx(), o, false)
	if err != nil || len(set.Own) != 32 || ok != 32 {
		t.Fatalf("count=%d success=%d err=%v", len(set.Own), ok, err)
	}
}

func TestLoweredQuotasPreserveReadsAndPermitNonGrowingEdits(t *testing.T) {
	old := limits.Path()
	t.Cleanup(func() {
		if old == "" {
			limits.Configure("")
		} else {
			limits.Configure(filepath.Dir(old))
		}
	})
	s := openSecretsDB(t)
	id := insertNote(t, s, "grandfather", nil, false)
	o := SecretOwner{SecretOwnerNote, id}
	for _, name := range []string{"one", "two", "three"} {
		mustValue(t, s, o, name, "int", "3")
	}
	limits.Configure(t.TempDir())
	n := int64(1)
	if err := limits.Change("values_own", &n); err != nil {
		t.Fatal(err)
	}
	if err := limits.Change("values_effective", &n); err != nil {
		t.Fatal(err)
	}
	set, err := s.ResolveNamedValues(valueCtx(), o, false)
	if err != nil || len(set.Own) != 3 || len(set.Effective) != 3 {
		t.Fatal("read lost existing values", set, err)
	}
	if _, err := s.SetNamedValue(valueCtx(), o, "one", "int", "4"); err != nil {
		t.Fatal("non-growing edit refused", err)
	}
	if _, err := s.SetNamedValue(valueCtx(), o, "four", "int", "1"); err == nil {
		t.Fatal("new name admitted over quota")
	}
	target := insertNote(t, s, "new copy", nil, false)
	if err := s.CopyNoteValues(valueCtx(), id, target, false); err == nil {
		t.Fatal("new oversized copy admitted")
	}
	set, err = s.ResolveNamedValues(valueCtx(), SecretOwner{SecretOwnerNote, target}, false)
	if err != nil || len(set.Own) != 0 {
		t.Fatal("partial copy", set, err)
	}
}
