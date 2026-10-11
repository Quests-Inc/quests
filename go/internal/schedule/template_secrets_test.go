package schedule

import (
	"testing"

	"github.com/quests-inc/quests/go/internal/store"
)

func openTemplateSecretsDB(t *testing.T) *store.Store {
	t.Helper()
	return openChecksDB(t)
}

func TestTemplateSecretsNeverListValues(t *testing.T) {
	st := openTemplateSecretsDB(t)
	const templateID = int64(1)
	if err := st.Automation().SetTemplateSecret(bg(), templateID, "PASSWORD", "hunter2"); err != nil {
		t.Fatalf("SetTemplateSecret: %v", err)
	}

	keys, err := st.Automation().ListTemplateSecretKeys(bg(), templateID)
	if err != nil {
		t.Fatalf("ListTemplateSecretKeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "PASSWORD" {
		t.Fatalf("want [PASSWORD], got %v", keys)
	}

	if err := st.Automation().DeleteTemplateSecret(bg(), templateID, "PASSWORD"); err != nil {
		t.Fatalf("DeleteTemplateSecret: %v", err)
	}
	resolved, err := st.Automation().ResolveTemplateSecrets(bg(), templateID)
	if err != nil {
		t.Fatalf("ResolveTemplateSecrets: %v", err)
	}
	if len(resolved) != 0 {
		t.Fatalf("want no secrets after delete, got %v", resolved)
	}
}
