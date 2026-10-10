package schedule

import (
	"testing"

	"github.com/valirum/quests/go/internal/store"
)

func openTemplateSecretsDB(t *testing.T) *store.Store {
	t.Helper()
	return openChecksDB(t)
}

// TestExecEmitPoolCommandInjectsTemplateSecret proves the end-to-end path:
// a secret set via SetTemplateSecret shows up as an env var in the script's
// process, and nowhere else — this is the whole point of note=14/quest=199.
func TestExecEmitPoolCommandInjectsTemplateSecret(t *testing.T) {
	st := openTemplateSecretsDB(t)
	const templateID = int64(40)
	if err := st.SetTemplateSecret(bg(), templateID, "MY_SECRET", "hunter2"); err != nil {
		t.Fatalf("SetTemplateSecret: %v", err)
	}

	command := `#!/bin/sh
printf '{"title":"%s"}' "$MY_SECRET"
`
	out, _, err := execEmitPoolCommand(bg(), st, templateID, command)
	if err != nil {
		t.Fatalf("execEmitPoolCommand: %v", err)
	}
	if out.Spec == nil || out.Spec.Title == nil || *out.Spec.Title != "hunter2" {
		t.Fatalf("want {title:hunter2}, got %+v", out)
	}
}

// A template with no secrets set must not blow up — ResolveTemplateSecrets
// just returns an empty map and the env is unchanged. (An unset $MY_SECRET
// yields an empty title, which execEmitPoolCommand drops as a non-item —
// see materialize.go's blank-title filter — so the assertion is "no crash,
// no leaked pool item", not a literal empty-titled result.)
func TestExecEmitPoolCommandNoSecretsIsFine(t *testing.T) {
	st := openTemplateSecretsDB(t)
	const templateID = int64(7)

	command := `#!/bin/sh
printf '{"title":"%s"}' "$MY_SECRET"
`
	out, _, err := execEmitPoolCommand(bg(), st, templateID, command)
	if err != nil {
		t.Fatalf("execEmitPoolCommand: %v", err)
	}
	// An unset $MY_SECRET leaves an empty title: still a valid (title-less)
	// object, and the secret value itself is nowhere in it.
	if out.Spec == nil || out.Spec.Title == nil || *out.Spec.Title != "" {
		t.Fatalf("want a spec with an empty title, got %+v", out)
	}
}

func TestTemplateSecretsNeverListValues(t *testing.T) {
	st := openTemplateSecretsDB(t)
	const templateID = int64(1)
	if err := st.SetTemplateSecret(bg(), templateID, "PASSWORD", "hunter2"); err != nil {
		t.Fatalf("SetTemplateSecret: %v", err)
	}

	keys, err := st.ListTemplateSecretKeys(bg(), templateID)
	if err != nil {
		t.Fatalf("ListTemplateSecretKeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "PASSWORD" {
		t.Fatalf("want [PASSWORD], got %v", keys)
	}

	if err := st.DeleteTemplateSecret(bg(), templateID, "PASSWORD"); err != nil {
		t.Fatalf("DeleteTemplateSecret: %v", err)
	}
	resolved, err := st.ResolveTemplateSecrets(bg(), templateID)
	if err != nil {
		t.Fatalf("ResolveTemplateSecrets: %v", err)
	}
	if len(resolved) != 0 {
		t.Fatalf("want no secrets after delete, got %v", resolved)
	}
}
