package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/limits"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// ValueInput is a manual typed upsert: exactly one of value or formula is required.
type ValueInput struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Value   json.RawMessage `json:"value,omitempty"`
	Formula *string         `json:"formula,omitempty"`
}

type ValueBatchError struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

func (e *ValueBatchError) Error() string {
	return fmt.Sprintf("values[%d] %s: %s", e.Index, e.Name, e.Message)
}

// SetNamedValues validates the final environment, independent of input order, then commits all own changes.
// Owner locking is shared by manual, script and rolled writes. No script is executed here.
func (s *Store) SetNamedValues(ctx context.Context, o SecretOwner, input []ValueInput, hidePrivate bool) ([]StoredValue, error) {
	return s.upsertValues(ctx, o, input, hidePrivate, 0, "", true)
}
func (s *Store) upsertValues(ctx context.Context, o SecretOwner, input []ValueInput, hidePrivate bool, roll int64, arg string, strictFinal bool) ([]StoredValue, error) {
	if len(input) == 0 || len(input) > int(limits.Specs["values_own"].Max) {
		return nil, &ValueError{Msg: "нужен непустой пакет в пределах лимита собственных переменных"}
	}
	changed := map[string]int{}
	values := make([]StoredValue, len(input))
	bad := func(i int, msg string) error { return &ValueBatchError{Index: i, Name: input[i].Name, Message: msg} }
	for i, row := range input {
		if err := CheckValueName(row.Name); err != nil {
			return nil, bad(i, err.Error())
		}
		if _, ok := changed[row.Name]; ok {
			return nil, bad(i, "повтор имени в пакете")
		}
		changed[row.Name] = i
		if (row.Formula != nil) == (len(row.Value) > 0) {
			return nil, bad(i, "нужно ровно одно из value и formula")
		}
		v := StoredValue{Name: row.Name, Type: row.Type, RollScript: roll, RollArg: arg}
		if row.Formula != nil {
			v.Formula = strings.TrimSpace(*row.Formula)
			if v.Formula == "" {
				return nil, bad(i, "нужна непустая формула")
			}
			switch row.Type {
			case "int", "float", "str", "bool":
			default:
				return nil, bad(i, "тип: int, float, str или bool")
			}
		} else {
			canonical, err := ParseNamedValue(row.Type, row.Value)
			if err != nil {
				return nil, bad(i, err.Error())
			}
			v.Value = canonical
		}
		values[i] = v
	}
	col, err := o.column()
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	scoped := *s
	scoped.valueTx = tx
	s = &scoped
	tables := map[string]string{"note": "note", "quest": "quest", "step": "queststep", "template": "questtemplate", "questline": "questline"}
	if table := tables[o.Kind]; table != "" {
		var id int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM "+table+" WHERE id=? FOR UPDATE", o.ID).Scan(&id); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(?))", fmt.Sprintf("values:workspace:%d", workspaceOwner(ctx).ID)); err != nil {
			return nil, err
		}
	}
	if hidePrivate && o.Kind == "note" {
		priv, err := s.NotePrivate(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		if priv {
			return nil, ErrPrivateValue
		}
	}
	set, err := s.collectNamedValues(ctx, o, hidePrivate)
	if err != nil {
		return nil, err
	}
	oldBytes := 0
	for _, v := range set.Own {
		oldBytes += len(v.Name) + len(v.Value) + len(v.Formula) + len(v.RollArg)
	}
	oldCount := len(set.Own)
	links, err := s.valueLinks(ctx, o)
	if err != nil {
		return nil, err
	}

	oldEffective := len(set.Effective)
	own := map[string]StoredValue{}
	for _, v := range set.Own {
		own[v.Name] = v
	}
	for _, v := range values {
		own[v.Name] = v
	}
	if o.Kind == SecretOwnerNote && len(links)-1 > int(limits.Get("values_chain")) && len(own) > oldCount {
		return nil, &ValueError{Msg: "превышена глубина наследования переменных"}
	}
	if len(own) > int(limits.Get("values_own")) && len(own) > oldCount {
		return nil, &ValueError{Msg: "превышен лимит собственных переменных"}
	}
	effective := map[string]ValuePlace{}
	for _, v := range set.Effective {
		effective[v.Name] = v
	}
	for _, v := range own {
		effective[v.Name] = ValuePlace{StoredValue: v, From: o}
	}
	if (len(effective) > int(limits.Get("values_effective")) && len(effective) > oldEffective) || set.Truncated {
		return nil, &ValueError{Msg: "превышен лимит эффективных переменных"}
	}
	set.Effective = nil
	set.Own = nil
	for _, v := range effective {
		set.Effective = append(set.Effective, v)
	}
	for _, v := range own {
		set.Own = append(set.Own, v)
	}
	set = evalSet(set)
	total := 0
	evaluated := map[string]StoredValue{}
	for _, v := range set.Effective {
		if v.Error != "" {
			i, ok := changed[v.Name]
			if !strictFinal && !ok {
				evaluated[v.Name] = v.StoredValue
				continue
			}
			if ok {
				return nil, bad(i, v.Error)
			}
			return nil, &ValueBatchError{Index: -1, Name: v.Name, Message: v.Error}
		}
		evaluated[v.Name] = v.StoredValue
	}
	for _, v := range set.Own {
		if _, ok := changed[v.Name]; !ok {
			v = own[v.Name]
		}
		total += len(v.Name) + len(v.Value) + len(v.Formula) + len(v.RollArg)
	}
	if total > int(limits.Get("values_bytes")) && total > oldBytes {
		return nil, &ValueError{Msg: "превышен байтовый лимит переменных"}
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	owner, ownerArgs, conflict := "?", []any{o.ID}, fmt.Sprintf("%[1]s, name) WHERE %[1]s IS NOT NULL", col)
	if o.Kind == SecretOwnerWorkspace {
		owner = "1"
		ownerArgs = nil
		conflict = "workspace_id, name) WHERE workspace_scope = 1"
	}
	for i, v := range values {
		v = evaluated[v.Name]
		values[i] = v
		var script any
		if v.RollScript > 0 {
			script = v.RollScript
		}
		_, err = tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO named_value (%s,name,type,value,formula,roll_script,roll_arg,created_at,updated_at) VALUES (%s,?,?,?,?,?,?,?,?) ON CONFLICT(%s DO UPDATE SET type=excluded.type,value=excluded.value,formula=excluded.formula,roll_script=excluded.roll_script,roll_arg=excluded.roll_arg,updated_at=excluded.updated_at`, col, owner, conflict), append(append([]any{}, ownerArgs...), v.Name, v.Type, v.Value, v.Formula, script, v.RollArg, now, now)...)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return values, nil
}
