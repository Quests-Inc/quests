package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/valirum/quests/go/internal/timeutil"
)

// Secret owners. A secret is attached to exactly one of these; values are
// resolved from the most specific owner to the most general one:
//
//	step > quest > template > questline
//
// i.e. a command sees its own secrets, then those of its quest, of the template
// that emitted the quest, and of the questline the quest sits in. Values are
// stored as plaintext (the server is the trust boundary — the commands run on
// the same host) and are never returned by a read endpoint or tool.
const (
	SecretOwnerQuestline = "questline"
	SecretOwnerTemplate  = "template"
	SecretOwnerQuest     = "quest"
	SecretOwnerStep      = "step"
)

// SecretOwner identifies what a secret is attached to.
type SecretOwner struct {
	Kind string
	ID   int64
}

// SecretRef is a secret name with the owner it comes from.
type SecretRef struct {
	Key   string
	Owner SecretOwner
}

var errBadSecretOwner = errors.New("unknown secret owner kind")

func (o SecretOwner) column() (string, error) {
	switch o.Kind {
	case SecretOwnerQuestline:
		return "questline_id", nil
	case SecretOwnerTemplate:
		return "template_id", nil
	case SecretOwnerQuest:
		return "quest_id", nil
	case SecretOwnerStep:
		return "step_id", nil
	}
	return "", fmt.Errorf("%w: %q", errBadSecretOwner, o.Kind)
}

// SecretOwnerExists reports whether the owning row exists.
func (s *Store) SecretOwnerExists(ctx context.Context, o SecretOwner) (bool, error) {
	var table string
	switch o.Kind {
	case SecretOwnerQuestline:
		table = "questline"
	case SecretOwnerTemplate:
		table = "questtemplate"
	case SecretOwnerQuest:
		table = "quest"
	case SecretOwnerStep:
		table = "queststep"
	default:
		return false, fmt.Errorf("%w: %q", errBadSecretOwner, o.Kind)
	}
	var one int
	err := s.DB.QueryRowContext(ctx, "SELECT 1 FROM "+table+" WHERE id = ?", o.ID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// sealedPrefix marks a value sealed by secretbox. Rows from before sealing hold the bare value.
const sealedPrefix = "v1:"

func (s *Store) sealSecret(v string) (string, error) {
	if s.Box == nil {
		return v, nil
	}
	return s.Box.Seal(v)
}

// openSecret returns the plain value of a stored one; ok=false means it cannot be opened
// (sealed under another key), and the secret is left out rather than handed over as ciphertext.
func (s *Store) openSecret(stored string) (string, bool) {
	if s.Box == nil || !strings.HasPrefix(stored, sealedPrefix) {
		return stored, true // not sealed: a legacy plain row (sealed at the next start)
	}
	plain, err := s.Box.Open(stored)
	if err != nil {
		return "", false
	}
	return plain, true
}

// SealLegacySecrets seals values stored in the clear, in every workspace. It runs at start;
// a value that merely looks sealed but cannot be opened is a plain value that begins with "v1:".
func (s *Store) SealLegacySecrets(ctx context.Context) (int, error) {
	if s.Box == nil {
		return 0, nil
	}
	total := 0
	var failure error
	err := s.ForEachWorkspace(ctx, func(wctx context.Context, _ int64) {
		rows, err := s.DB.QueryContext(wctx, `SELECT id, value FROM secret`)
		if err != nil {
			failure = err
			return
		}
		type row struct {
			id int64
			v  string
		}
		var todo []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.v); err != nil {
				failure = err
				break
			}
			if strings.HasPrefix(r.v, sealedPrefix) {
				if _, err := s.Box.Open(r.v); err == nil {
					continue
				}
			}
			todo = append(todo, r)
		}
		rows.Close()
		for _, r := range todo {
			sealed, err := s.Box.Seal(r.v)
			if err == nil {
				_, err = s.DB.ExecContext(wctx, `UPDATE secret SET value = ? WHERE id = ?`, sealed, r.id)
			}
			if err != nil {
				failure = err
				return
			}
			total++
		}
	})
	if err != nil {
		return total, err
	}
	return total, failure
}

// SetSecret upserts one secret value for an owner.
func (s *Store) SetSecret(ctx context.Context, o SecretOwner, key, value string) error {
	value, err := s.sealSecret(value)
	if err != nil {
		return err
	}
	col, err := o.column()
	if err != nil {
		return err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO secret (%[1]s, key, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(%[1]s, key) WHERE %[1]s IS NOT NULL
		DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, col),
		o.ID, key, value, now, now)
	return err
}

// ListSecretKeys returns the names set directly on the owner — never values.
func (s *Store) ListSecretKeys(ctx context.Context, o SecretOwner) ([]string, error) {
	col, err := o.column()
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, fmt.Sprintf(`SELECT key FROM secret WHERE %s = ? ORDER BY key`, col), o.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteSecret removes one key; a missing key is not an error.
func (s *Store) DeleteSecret(ctx context.Context, o SecretOwner, key string) error {
	col, err := o.column()
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`DELETE FROM secret WHERE %s = ? AND key = ?`, col), o.ID, key)
	return err
}

// secretChain returns the owners whose secrets apply to o, least specific
// first (so a later owner overrides an earlier one).
func (s *Store) secretChain(ctx context.Context, o SecretOwner) ([]SecretOwner, error) {
	var questlineID, templateID, questID sql.NullInt64
	switch o.Kind {
	case SecretOwnerQuestline:
		return []SecretOwner{o}, nil
	case SecretOwnerTemplate:
		err := s.DB.QueryRowContext(ctx, `SELECT questline_id FROM questtemplate WHERE id = ?`, o.ID).Scan(&questlineID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	case SecretOwnerQuest, SecretOwnerStep:
		qid := o.ID
		if o.Kind == SecretOwnerStep {
			err := s.DB.QueryRowContext(ctx, `SELECT quest_id FROM queststep WHERE id = ?`, o.ID).Scan(&questID)
			if errors.Is(err, sql.ErrNoRows) {
				return []SecretOwner{o}, nil
			}
			if err != nil {
				return nil, err
			}
			qid = questID.Int64
		}
		err := s.DB.QueryRowContext(ctx, `SELECT questline_id, template_id FROM quest WHERE id = ?`, qid).Scan(&questlineID, &templateID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if o.Kind == SecretOwnerStep {
			questID = sql.NullInt64{Int64: qid, Valid: true}
		}
	default:
		return nil, fmt.Errorf("%w: %q", errBadSecretOwner, o.Kind)
	}

	var chain []SecretOwner
	if questlineID.Valid {
		chain = append(chain, SecretOwner{SecretOwnerQuestline, questlineID.Int64})
	}
	if o.Kind != SecretOwnerTemplate && templateID.Valid {
		chain = append(chain, SecretOwner{SecretOwnerTemplate, templateID.Int64})
	}
	switch o.Kind {
	case SecretOwnerStep:
		chain = append(chain, SecretOwner{SecretOwnerQuest, questID.Int64})
	}
	chain = append(chain, o)
	return chain, nil
}

func (s *Store) secretValues(ctx context.Context, o SecretOwner) (map[string]string, error) {
	col, err := o.column()
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, fmt.Sprintf(`SELECT key, value FROM secret WHERE %s = ?`, col), o.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		if plain, ok := s.openSecret(v); ok {
			out[k] = plain
		}
	}
	return out, rows.Err()
}

// ResolveSecrets returns the effective secrets for an owner — its own plus
// everything inherited — keyed by name. Callers inject these into a command's
// environment and mask them in whatever the command prints; nothing else may
// read values.
func (s *Store) ResolveSecrets(ctx context.Context, o SecretOwner) (map[string]string, error) {
	chain, err := s.secretChain(ctx, o)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, owner := range chain {
		vals, err := s.secretValues(ctx, owner)
		if err != nil {
			return nil, err
		}
		for k, v := range vals {
			if ReservedSecretName(k) {
				continue // rows that predate the rule (or came from an import) are never exported to a process
			}
			out[k] = v
		}
	}
	return out, nil
}

// ListEffectiveSecretKeys returns the names that apply to an owner, each with
// the owner it effectively comes from (the most specific one) — never values.
func (s *Store) ListEffectiveSecretKeys(ctx context.Context, o SecretOwner) ([]SecretRef, error) {
	chain, err := s.secretChain(ctx, o)
	if err != nil {
		return nil, err
	}
	src := map[string]SecretOwner{}
	for _, owner := range chain {
		keys, err := s.ListSecretKeys(ctx, owner)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			src[k] = owner
		}
	}
	out := make([]SecretRef, 0, len(src))
	for k, owner := range src {
		out = append(out, SecretRef{Key: k, Owner: owner})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// MaskSecrets replaces every secret value in s with "***", so a command that
// echoes a credential can't leak it into a log, a step description or a trace.
// Values shorter than 4 characters are left alone (too likely to be ordinary
// text). Encoding tricks (base64, splitting) are not caught.
func MaskSecrets(s string, secrets map[string]string) string {
	for _, v := range secrets {
		if len(strings.TrimSpace(v)) >= 4 {
			s = strings.ReplaceAll(s, v, "***")
		}
	}
	return s
}

// --- template-specific wrappers (the original API) ---

func (s *Store) SetTemplateSecret(ctx context.Context, templateID int64, name, value string) error {
	return s.SetSecret(ctx, SecretOwner{SecretOwnerTemplate, templateID}, name, value)
}

func (s *Store) ListTemplateSecretKeys(ctx context.Context, templateID int64) ([]string, error) {
	return s.ListSecretKeys(ctx, SecretOwner{SecretOwnerTemplate, templateID})
}

func (s *Store) DeleteTemplateSecret(ctx context.Context, templateID int64, name string) error {
	return s.DeleteSecret(ctx, SecretOwner{SecretOwnerTemplate, templateID}, name)
}

// ResolveTemplateSecrets is what an emit_pool_command sees: the template's own
// secrets plus those of its questline.
func (s *Store) ResolveTemplateSecrets(ctx context.Context, templateID int64) (map[string]string, error) {
	return s.ResolveSecrets(ctx, SecretOwner{SecretOwnerTemplate, templateID})
}

// copyTemplateSecretsToLiveQuests keeps the check commands of quests a template
// emitted working after the template is deleted: quest.template_id is nulled,
// so the inherited secrets are copied onto the quests that are still open and
// have an auto-check. Called inside DeleteTemplate's transaction.
func copyTemplateSecretsToLiveQuests(ctx context.Context, tx *sql.Tx, templateID int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO secret (quest_id, key, value, created_at, updated_at)
		SELECT q.id, s.key, s.value, s.created_at, s.updated_at
		FROM quest q JOIN secret s ON s.template_id = ?
		WHERE q.template_id = ? AND q.status IN ('active', 'frozen', 'expired')
		  AND EXISTS (
		    SELECT 1 FROM queststep st
		    WHERE st.quest_id = q.id AND st.check_command IS NOT NULL AND TRIM(st.check_command) != '')
		ON CONFLICT DO NOTHING`,
		templateID, templateID)
	return err
}

// ReservedSecretName reports names that change how a process starts rather than carry a
// credential. Secrets become environment variables of commands (on the server or a
// runner), so e.g. PATH, LD_PRELOAD or BASH_ENV would let whoever can set a secret
// bend a command someone else wrote.
func ReservedSecretName(key string) bool {
	k := strings.ToUpper(key)
	for _, prefix := range []string{"LD_", "DYLD_", "QUESTS_", "PYTHON", "NODE_", "BASH_", "GIT_", "SSH_", "XDG_", "LC_"} {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	switch k {
	case "PATH", "HOME", "SHELL", "IFS", "ENV", "SHELLOPTS", "BASHOPTS", "PS1", "PS2", "PS4", "PROMPT_COMMAND",
		"CDPATH", "GLOBIGNORE", "LANG", "TMPDIR", "USER", "LOGNAME", "PWD", "OLDPWD", "TERM", "EDITOR", "VISUAL",
		"PAGER", "BROWSER", "PERL5OPT", "PERL5LIB", "RUBYOPT", "RUBYLIB", "JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS", "CLASSPATH":
		return true
	}
	return false
}
