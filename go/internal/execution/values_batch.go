package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// WriteScriptValues writes a JSON object's scalar fields atomically, without replacing formulas.
// Nothing is written when any key, type, formula or total size fails validation.
func (s *Store) WriteScriptValues(ctx context.Context, o SecretOwner, raw json.RawMessage) ([]StoredValue, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) == 0 || len(fields) > maxValuesPerOwner {
		return nil, &ValueError{Msg: "нужен непустой объект, не больше 32 переменных"}
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]StoredValue, 0, len(names))
	for _, name := range names {
		if err := CheckValueName(name); err != nil {
			return nil, err
		}
		v := bytes.TrimSpace(fields[name])
		typ := "float"
		switch {
		case len(v) == 0:
			return nil, &ValueError{Msg: "пустое значение"}
		case v[0] == '"':
			typ = "str"
		case string(v) == "true" || string(v) == "false":
			typ = "bool"
		case !bytes.ContainsAny(v, ".eE"):
			typ = "int"
		}
		canonical, err := ParseNamedValue(typ, v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		values = append(values, StoredValue{Name: name, Type: typ, Value: canonical})
	}
	cond, args, err := o.match()
	if err != nil {
		return nil, err
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
	// Lock the owning row so concurrent batch writes on an empty environment serialize too.
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
	rows, err := tx.QueryContext(ctx, `SELECT name, value, formula, roll_arg FROM named_value WHERE `+cond+` FOR UPDATE`, args...)
	if err != nil {
		return nil, err
	}
	merged := map[string]int{}
	formulas := map[string]bool{}
	for rows.Next() {
		var name, value, formula, arg string
		if err := rows.Scan(&name, &value, &formula, &arg); err != nil {
			rows.Close()
			return nil, err
		}
		merged[name] = len(name) + len(value) + len(formula) + len(arg)
		formulas[name] = formula != ""
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		if formulas[v.Name] {
			return nil, &ValueError{Msg: "«" + v.Name + "» — формула: запись её не заменяет"}
		}
		merged[v.Name] = len(v.Name) + len(v.Value)
	}
	size := 0
	for _, n := range merged {
		size += n
	}
	if len(merged) > maxValuesPerOwner || size > maxValueBytes {
		return nil, &ValueError{Msg: "не больше 32 переменных и 8 КБ на сущность"}
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	owner, ownerArgs, conflict := "?", []any{o.ID}, fmt.Sprintf("%[1]s, name) WHERE %[1]s IS NOT NULL", col)
	if o.Kind == SecretOwnerWorkspace {
		owner = "1"
		ownerArgs = nil
		conflict = "workspace_id, name) WHERE workspace_scope = 1"
	}
	for _, v := range values {
		result, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO named_value (%s,name,type,value,formula,roll_script,roll_arg,created_at,updated_at) VALUES (%s,?,?,?,'',NULL,'',?,?) ON CONFLICT(%s DO UPDATE SET type=excluded.type,value=excluded.value,roll_script=NULL,roll_arg='',updated_at=excluded.updated_at WHERE named_value.formula=''`, col, owner, conflict), append(append([]any{}, ownerArgs...), v.Name, v.Type, v.Value, now, now)...)
		if err != nil {
			return nil, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, &ValueError{Msg: "формула изменилась во время записи"}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return values, nil
}
