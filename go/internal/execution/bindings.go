package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// Kinds of subject a script can be bound to.
const (
	SubjectStep = "step"
	SubjectNote = "note"
	// SubjectTemplate: a template's emit script (it replaces emit_pool_command).
	SubjectTemplate = "template"
)

// Binding is the script a subject runs, at its current version.
type Binding struct {
	ScriptID    int64
	Version     int
	NeedsRunner bool
}

// ScriptUse is a place a script is bound to.
type ScriptUse struct {
	SubjectType string `json:"subject_type"`
	SubjectID   int64  `json:"subject_id"`
	Title       string `json:"title"`
}

// ScriptInUse is returned when a script that is still bound is deleted.
type ScriptInUse struct{ Uses []ScriptUse }

func (e *ScriptInUse) Error() string {
	return fmt.Sprintf("скрипт используется (%d): сначала отвяжите его", len(e.Uses))
}

func validSubject(kind string) bool {
	return kind == SubjectStep || kind == SubjectNote || kind == SubjectTemplate
}

// BindScript makes a subject run a script. The script must read only that kind of entity (a script built on a note does not
// fit a step), so a mismatch is caught here and not in a run.
func (s *Store) BindScript(ctx context.Context, subjectType string, subjectID, scriptID int64) error {
	if !validSubject(subjectType) {
		return errBad("скрипт можно привязать к шагу, заметке или шаблону")
	}
	sc, err := s.GetScript(ctx, scriptID)
	if err != nil {
		return err
	}
	if len(sc.Subjects) != 1 || sc.Subjects[0] != subjectType {
		return errBad(fmt.Sprintf("скрипт читает %s, а привязывается к %s", strings.Join(sc.Subjects, ", "), subjectType))
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	_, err = s.DB.ExecContext(ctx, `INSERT INTO script_binding (subject_type, subject_id, script_id, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (workspace_id, subject_type, subject_id) DO UPDATE SET script_id = excluded.script_id, created_at = excluded.created_at`,
		subjectType, subjectID, scriptID, now)
	return err
}

// UnbindScript removes a subject's binding; there being none is not an error.
func (s *Store) UnbindScript(ctx context.Context, subjectType string, subjectID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM script_binding WHERE subject_type = ? AND subject_id = ?`, subjectType, subjectID)
	return err
}

const bindingSelect = `SELECT b.subject_id, b.script_id, s.version, s.needs_runner FROM script_binding b JOIN script s ON s.id = b.script_id`

// BindingFor returns the binding of one subject, or nil.
func (s *Store) BindingFor(ctx context.Context, subjectType string, subjectID int64) (*Binding, error) {
	var id int64
	var b Binding
	var needs int
	err := s.DB.QueryRowContext(ctx, bindingSelect+` WHERE b.subject_type = ? AND b.subject_id = ?`, subjectType, subjectID).
		Scan(&id, &b.ScriptID, &b.Version, &needs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.NeedsRunner = needs == 1
	return &b, nil
}

// Bindings returns every binding of one kind of subject, by subject id: one query for a scheduler tick.
func (s *Store) Bindings(ctx context.Context, subjectType string) (map[int64]Binding, error) {
	rows, err := s.DB.QueryContext(ctx, bindingSelect+` WHERE b.subject_type = ?`, subjectType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Binding{}
	for rows.Next() {
		var id int64
		var b Binding
		var needs int
		if err := rows.Scan(&id, &b.ScriptID, &b.Version, &needs); err != nil {
			return nil, err
		}
		b.NeedsRunner = needs == 1
		out[id] = b
	}
	return out, rows.Err()
}

// ScriptUses lists where a script is bound, with the title of the step or note.
func (s *Store) ScriptUses(ctx context.Context, scriptID int64) ([]ScriptUse, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT b.subject_type, b.subject_id,
		       COALESCE((SELECT title FROM queststep WHERE id = b.subject_id AND b.subject_type = 'step'),
		                (SELECT title FROM note WHERE id = b.subject_id AND b.subject_type = 'note'),
		                (SELECT title FROM questtemplate WHERE id = b.subject_id AND b.subject_type = 'template'), '')
		FROM script_binding b WHERE b.script_id = ?
		UNION ALL
		SELECT 'template_step', id, title FROM questtemplatestep WHERE script_id = ?
		ORDER BY 1, 2`, scriptID, scriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScriptUse{}
	for rows.Next() {
		var u ScriptUse
		if err := rows.Scan(&u.SubjectType, &u.SubjectID, &u.Title); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// PurgeOrphanBindings drops the bindings whose step or note no longer exists.
func (s *Store) PurgeOrphanBindings(ctx context.Context) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM script_binding
		WHERE (subject_type = 'step' AND NOT EXISTS (SELECT 1 FROM queststep WHERE id = script_binding.subject_id))
		   OR (subject_type = 'note' AND NOT EXISTS (SELECT 1 FROM note WHERE id = script_binding.subject_id))
		   OR (subject_type = 'template' AND NOT EXISTS (SELECT 1 FROM questtemplate WHERE id = script_binding.subject_id))`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
