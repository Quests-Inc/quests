package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func valueCtx() context.Context { return wsctx.With(context.Background(), 1) }

func insertNote(t *testing.T, st *Store, title string, parent *int64, private bool) int64 {
	t.Helper()
	priv := 0
	if private {
		priv = 1
	}
	res, err := st.DB.ExecContext(valueCtx(), `INSERT INTO note (title, parent_id, is_private, created_at, updated_at) VALUES (?, ?, ?, '2026-01-01', '2026-01-01')`, title, parent, priv)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustValue(t *testing.T, st *Store, o SecretOwner, name, typ, canonical string) {
	t.Helper()
	if _, err := st.SetNamedValue(valueCtx(), o, name, typ, canonical); err != nil {
		t.Fatalf("set %s: %v", name, err)
	}
}

func TestNamedValueInheritsNoteThenWorkspace(t *testing.T) {
	st := openSecretsDB(t)
	ctx := valueCtx()
	parent := insertNote(t, st, "герой", nil, false)
	child := insertNote(t, st, "сцена", &parent, false)
	mustValue(t, st, SecretOwner{SecretOwnerNote, parent}, "str", "int", "3")
	mustValue(t, st, SecretOwner{SecretOwnerNote, child}, "str", "int", "5")
	mustValue(t, st, SecretOwner{SecretOwnerWorkspace, wsctx.From(ctx)}, "dc", "int", "15")
	mustValue(t, st, SecretOwner{SecretOwnerNote, parent}, "repo", "str", "quests")

	set, err := st.ResolveNamedValues(ctx, SecretOwner{SecretOwnerNote, child}, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range set.Effective {
		got[p.Name] = p.Value
	}
	if got["str"] != "5" || got["dc"] != "15" || got["repo"] != "quests" {
		t.Fatalf("effective = %v", got)
	}
	if len(set.Own) != 1 || set.Own[0].Name != "str" {
		t.Fatalf("own = %+v", set.Own)
	}
	env := set.Env()
	if env["STR"] != "5" || env["DC"] != "15" || env["REPO"] != "quests" {
		t.Fatalf("env = %v", env)
	}
}

func TestNamedValueSkipsPrivateAncestorForMCP(t *testing.T) {
	st := openSecretsDB(t)
	parent := insertNote(t, st, "тайна", nil, true)
	child := insertNote(t, st, "явное", &parent, false)
	mustValue(t, st, SecretOwner{SecretOwnerNote, parent}, "hid", "int", "1")
	mustValue(t, st, SecretOwner{SecretOwnerWorkspace, 1}, "dc", "int", "12")

	set, err := st.ResolveNamedValues(valueCtx(), SecretOwner{SecretOwnerNote, child}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range set.Effective {
		if p.Name == "hid" {
			t.Fatal("private ancestor leaked")
		}
	}
	if set.Env()["DC"] != "12" {
		t.Fatalf("workspace value missing: %+v", set.Effective)
	}
}

func TestNamedValueCycleStops(t *testing.T) {
	st := openSecretsDB(t)
	a := insertNote(t, st, "a", nil, false)
	b := insertNote(t, st, "b", &a, false)
	if _, err := st.DB.ExecContext(valueCtx(), `UPDATE note SET parent_id = ? WHERE id = ?`, b, a); err != nil {
		t.Fatal(err)
	}
	mustValue(t, st, SecretOwner{SecretOwnerNote, a}, "froma", "int", "1")
	mustValue(t, st, SecretOwner{SecretOwnerNote, b}, "fromb", "int", "2")
	set, err := st.ResolveNamedValues(valueCtx(), SecretOwner{SecretOwnerNote, a}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Effective) < 2 {
		t.Fatalf("cycle ate values: %+v", set.Effective)
	}
}

func TestNamedValueRejects(t *testing.T) {
	st := openSecretsDB(t)
	o := SecretOwner{SecretOwnerNote, insertNote(t, st, "n", nil, false)}
	for _, name := range []string{"Str", "expr", "true", "false", "path", "quests_job"} {
		if _, err := st.SetNamedValue(valueCtx(), o, name, "int", "1"); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if _, err := st.SetNamedValue(valueCtx(), o, "n", "int", "1.5"); err == nil {
		t.Fatal("fractional int")
	}
	long := strings.Repeat("a", maxValueStr+1)
	if _, err := st.SetNamedValue(valueCtx(), o, "s", "str", long); err == nil {
		t.Fatal("long string")
	}
	for i := range maxValuesPerOwner {
		mustValue(t, st, o, "n"+strconv.Itoa(i), "int", "1")
	}
	if _, err := st.SetNamedValue(valueCtx(), o, "overflow", "int", "1"); err == nil {
		t.Fatal("33rd value")
	}
}

func TestNamedValueStepSeesQuestAndWorkspace(t *testing.T) {
	st := openSecretsDB(t)
	ctx := valueCtx()
	mustValue(t, st, SecretOwner{SecretOwnerWorkspace, 1}, "dc", "int", "10")
	mustValue(t, st, oQuest, "dc", "int", "14")
	mustValue(t, st, oStep, "ref", "str", "abc")
	set, err := st.ResolveNamedValues(ctx, oStep, false)
	if err != nil {
		t.Fatal(err)
	}
	got := set.Env()
	if got["DC"] != "14" || got["REF"] != "abc" {
		t.Fatalf("env = %v", got)
	}
}

func TestNamedFormulaFollowsStat(t *testing.T) {
	st := openSecretsDB(t)
	parent := insertNote(t, st, "герой", nil, false)
	child := insertNote(t, st, "волк", &parent, false)
	po := SecretOwner{SecretOwnerNote, parent}
	co := SecretOwner{SecretOwnerNote, child}
	mustValue(t, st, po, "score", "int", "12")
	if _, err := st.SetNamedFormula(valueCtx(), po, "mod", "int", "(score - 10) / 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetNamedFormula(valueCtx(), po, "mod", "int", "mod + 1"); err == nil {
		t.Fatal("cycle accepted")
	}
	if _, err := st.SetNamedFormula(valueCtx(), po, "bad", "int", "1 / 0"); err == nil {
		t.Fatal("division by zero accepted")
	}
	set, err := st.ResolveNamedValues(valueCtx(), po, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := valueMap(set); got["mod"] != "1" || got["score"] != "12" {
		t.Fatalf("parent = %v", got)
	}
	mustValue(t, st, po, "score", "int", "14")
	set, err = st.ResolveNamedValues(valueCtx(), po, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := valueMap(set); got["mod"] != "2" {
		t.Fatalf("stat change did not recompute: %v", got)
	}
	mustValue(t, st, co, "score", "int", "9")
	set, err = st.ResolveNamedValues(valueCtx(), co, false)
	if err != nil {
		t.Fatal(err)
	}
	got := valueMap(set)
	if got["score"] != "9" || got["mod"] != "-1" {
		t.Fatalf("child = %v", got)
	}
	if set.Env()["MOD"] != "-1" {
		t.Fatalf("env = %v", set.Env())
	}
	var formula string
	for _, p := range set.Effective {
		if p.Name == "mod" {
			formula = p.Formula
		}
	}
	if formula != "(score - 10) / 2" {
		t.Fatalf("formula = %q", formula)
	}
}

func valueMap(set ValueSet) map[string]string {
	out := map[string]string{}
	for _, p := range set.Effective {
		if p.Error != "" {
			out[p.Name] = "err:" + p.Error
			continue
		}
		out[p.Name] = p.Value
	}
	return out
}

func TestParseNamedValue(t *testing.T) {
	n, err := ParseNamedValue("bool", []byte("true"))
	if err != nil || n != "true" {
		t.Fatal(n, err)
	}
	v := StoredValue{Name: "flag", Type: "bool", Value: "true"}
	k, ev := v.EnvPair()
	if k != "FLAG" || ev != "1" {
		t.Fatal(k, ev)
	}
	if _, err := ParseNamedValue("float", []byte("1e999")); err == nil {
		t.Fatal("inf")
	}
	if _, err := stMissing(t); err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func stMissing(t *testing.T) (bool, error) {
	t.Helper()
	st := openSecretsDB(t)
	return st.NotePrivate(valueCtx(), 999999)
}

func TestScriptValueWriteKeepsFormulas(t *testing.T) {
	st := openSecretsDB(t)
	ctx := valueCtx()
	note := insertNote(t, st, "волк", nil, false)
	o := SecretOwner{SecretOwnerNote, note}
	mustValue(t, st, o, "str_score", "int", "12")
	if err := st.WriteScriptValue(ctx, o, "hp", "int", "11"); err != nil {
		t.Fatalf("a new value: %v", err)
	}
	if err := st.WriteScriptValue(ctx, o, "hp", "int", "9"); err != nil {
		t.Fatalf("an existing literal: %v", err)
	}
	if _, err := st.SetNamedFormula(ctx, o, "str_mod", "int", "(str_score - 10) / 2"); err != nil {
		t.Fatal(err)
	}
	err := st.WriteScriptValue(ctx, o, "str_mod", "int", "5")
	var ve *ValueError
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, "формула") {
		t.Fatalf("a formula must not be overwritten by a script: %v", err)
	}
	if err := st.WriteScriptValue(ctx, o, "hp", "int", "not a number"); err == nil {
		t.Fatal("a value that does not fit the type must be refused")
	}
}

func addRunner(t *testing.T, st *Store, label string) {
	t.Helper()
	reg, err := st.CreateRunner(valueCtx(), label)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RegisterRunner(valueCtx(), reg, "host", "test"); err != nil {
		t.Fatal(err)
	}
}

func TestEmbedRunnerChoice(t *testing.T) {
	st := openSecretsDB(t)
	ctx := valueCtx()
	ws := SecretOwner{SecretOwnerWorkspace, 1}
	if _, err := st.ResolveEmbedRunner(ctx, ""); err == nil {
		t.Fatal("no runner at all must be said")
	}
	addRunner(t, st, "main-pc")
	if got, err := st.ResolveEmbedRunner(ctx, ""); err != nil || got != "main-pc" {
		t.Fatalf("the only runner: %q %v", got, err)
	}
	addRunner(t, st, "laptop")
	addRunner(t, st, "vps")
	if _, err := st.ResolveEmbedRunner(ctx, ""); err == nil || !strings.Contains(err.Error(), "несколько") {
		t.Fatalf("several runners and no default: %v", err)
	}
	mustValue(t, st, ws, "runner", "str", "laptop, main-pc")
	if got, err := st.ResolveEmbedRunner(ctx, ""); err != nil || got != "laptop,main-pc" {
		t.Fatalf("the workspace chain keeps its order: %q %v", got, err)
	}
	if got, err := st.ResolveEmbedRunner(ctx, "vps"); err != nil || got != "vps" {
		t.Fatalf("the link overrides: %q %v", got, err)
	}
	if _, err := st.ResolveEmbedRunner(ctx, "prod-db"); err == nil || !strings.Contains(err.Error(), "не зарегистрирован") {
		t.Fatalf("a runner that is not ours: %v", err)
	}
	mustValue(t, st, ws, "runner", "str", "ghost")
	if _, err := st.ResolveEmbedRunner(ctx, ""); err == nil {
		t.Fatal("a default that names no runner must be refused")
	}
	if err := st.RevokeRunner(ctx, "vps"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ResolveEmbedRunner(ctx, "vps"); err == nil {
		t.Fatal("a revoked runner is not available")
	}
}

func TestScriptValuesBatchIsAtomic(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	o := oQuest
	values, err := st.WriteScriptValues(ctx, o, json.RawMessage(`{"hp":12,"ratio":1.5,"name":"wolf","alive":true}`))
	if err != nil || len(values) != 4 {
		t.Fatalf("write: %v %v", values, err)
	}
	if _, err := st.SetNamedFormula(ctx, o, "protected", "int", "hp+1"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"hp":99,"protected":3}`, `{"hp":99,"bad-key":1}`, `{"hp":99,"nested":{}}`, `{"hp":99,"nothing":null}`} {
		if _, err := st.WriteScriptValues(ctx, o, json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
		set, err := st.ResolveNamedValues(ctx, o, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range set.Own {
			if v.Name == "hp" && v.Value != "12" {
				t.Fatalf("partial write: %+v", set)
			}
		}
	}
}
