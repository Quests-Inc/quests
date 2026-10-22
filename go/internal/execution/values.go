package execution

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// Named values are readable data on an entity. A secret is the opposite: write-only, sealed, for a command's
// credentials. A value is shown, inherited, and may be substituted into an expression. A formula is computed from the
// other values in scope when the set is read, so a modifier follows the stat it names.
const (
	maxValuesPerOwner = 32
	maxValueBytes     = 8 << 10
	maxValueStr       = 500
	maxValueAbs       = 1_000_000_000.0
	maxValueChain     = 32
	maxEffective      = 64
)

// ValueError is a rejected name or literal. The API answers 422.
type ValueError struct{ Msg string }

func (e *ValueError) Error() string { return e.Msg }

// ErrPrivateValue is a private note read through MCP.
var ErrPrivateValue = errors.New("приватная заметка")

var valueNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,23}$`)

// CheckValueName accepts the same names as an embedded argument. The upper-case form is what a command sees, so a name
// that would become PATH or QUESTS_* is refused, and expr is the expression input.
func CheckValueName(name string) error {
	if !valueNameRe.MatchString(name) || name == "expr" || name == "true" || name == "false" {
		return &ValueError{Msg: "имя: строчные латинские, цифры и _, до 24 символов, не expr, true, false"}
	}
	if ReservedSecretName(name) {
		return &ValueError{Msg: "это имя занято: в команде оно меняет запуск, а не несёт значение"}
	}
	return nil
}

// ParseNamedValue reads one JSON literal into the canonical text stored in the table.
func ParseNamedValue(typ string, raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", &ValueError{Msg: "нужно значение"}
	}
	switch typ {
	case "int":
		if strings.ContainsAny(string(raw), ".eE") {
			return "", &ValueError{Msg: "нужно целое"}
		}
		var n int64
		if err := json.Unmarshal(raw, &n); err != nil || float64(n) > maxValueAbs || float64(n) < -maxValueAbs {
			return "", &ValueError{Msg: "целое вне диапазона"}
		}
		return strconv.FormatInt(n, 10), nil
	case "float":
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > maxValueAbs {
			return "", &ValueError{Msg: "дробное вне диапазона"}
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	case "bool":
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return "", &ValueError{Msg: "нужно да или нет"}
		}
		if b {
			return "true", nil
		}
		return "false", nil
	case "str":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", &ValueError{Msg: "нужна строка"}
		}
		if len(s) > maxValueStr {
			return "", &ValueError{Msg: "строка длиннее 500 символов"}
		}
		return s, nil
	default:
		return "", &ValueError{Msg: "тип: int, float, str или bool"}
	}
}

func normalizeValue(typ, canonical string) (string, error) {
	var raw json.RawMessage
	switch typ {
	case "str":
		b, err := json.Marshal(canonical)
		if err != nil {
			return "", err
		}
		raw = b
	case "int", "float", "bool":
		if strings.TrimSpace(canonical) == "" {
			return "", &ValueError{Msg: "нужно значение"}
		}
		raw = json.RawMessage(canonical)
	default:
		return "", &ValueError{Msg: "тип: int, float, str или bool"}
	}
	return ParseNamedValue(typ, raw)
}

// StoredValue is one value as it lies in the table. Formula is empty for a literal. Error is set when a formula
// cannot be computed; Value is then empty.
type StoredValue struct {
	Name    string
	Type    string
	Value   string
	Formula string
	Error   string
}

// Number is the value an expression can read. A string or a broken formula reports ok=false.
func (v StoredValue) Number() (float64, bool) {
	if v.Error != "" {
		return 0, false
	}
	switch v.Type {
	case "int", "float":
		n, err := strconv.ParseFloat(v.Value, 64)
		return n, err == nil
	case "bool":
		if v.Value == "true" {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// JSONValue is the literal in the API (a number, a bool or a string).
func (v StoredValue) JSONValue() (any, error) {
	switch v.Type {
	case "int":
		return strconv.ParseInt(v.Value, 10, 64)
	case "float":
		return strconv.ParseFloat(v.Value, 64)
	case "bool":
		return v.Value == "true", nil
	default:
		return v.Value, nil
	}
}

// EnvPair is the name a command sees (upper case) and the text in the environment. Bool is 1 or 0.
func (v StoredValue) EnvPair() (string, string) {
	key := strings.ToUpper(v.Name)
	if v.Type == "bool" {
		if v.Value == "true" {
			return key, "1"
		}
		return key, "0"
	}
	return key, v.Value
}

// ValuePlace is a stored literal plus the owner it was inherited from.
type ValuePlace struct {
	StoredValue
	From  SecretOwner
	Title string
}

// ValueSet is what a text or a command sees. Own is everything set on the owner itself. Effective is the inherited
// view, nearest first, capped.
type ValueSet struct {
	Own       []StoredValue
	Effective []ValuePlace
	Truncated bool
}

// Env is the effective values for a command. Names are upper case.
func (set ValueSet) Env() map[string]string {
	out := make(map[string]string, len(set.Effective))
	for _, p := range set.Effective {
		if p.Error != "" {
			continue
		}
		k, v := p.EnvPair()
		out[k] = v
	}
	return out
}

// SetNamedValue upserts one literal and clears any formula on that name.
func (s *Store) SetNamedValue(ctx context.Context, o SecretOwner, name, typ, canonical string) (StoredValue, error) {
	if err := CheckValueName(name); err != nil {
		return StoredValue{}, err
	}
	canonical, err := normalizeValue(typ, canonical)
	if err != nil {
		return StoredValue{}, err
	}
	return s.writeValue(ctx, o, name, typ, canonical, "")
}

// SetNamedFormula upserts a formula. It is checked against the values visible from o, including this name, so a
// cycle or a missing stat is rejected here. The computed text is what the table stores beside the formula.
func (s *Store) SetNamedFormula(ctx context.Context, o SecretOwner, name, typ, formula string) (StoredValue, error) {
	if err := CheckValueName(name); err != nil {
		return StoredValue{}, err
	}
	formula = strings.TrimSpace(formula)
	if formula == "" {
		return StoredValue{}, &ValueError{Msg: "нужна формула"}
	}
	switch typ {
	case "int", "float", "str", "bool":
	default:
		return StoredValue{}, &ValueError{Msg: "тип: int, float, str или bool"}
	}
	set, err := s.collectNamedValues(ctx, o, false)
	if err != nil {
		return StoredValue{}, err
	}
	placed := false
	for i := range set.Effective {
		if set.Effective[i].Name != name {
			continue
		}
		set.Effective[i].StoredValue = StoredValue{Name: name, Type: typ, Formula: formula}
		placed = true
		break
	}
	if !placed {
		set.Effective = append(set.Effective, ValuePlace{StoredValue: StoredValue{Name: name, Type: typ, Formula: formula}})
	}
	set = evalSet(set)
	var got StoredValue
	for _, p := range set.Effective {
		if p.Name == name {
			got = p.StoredValue
			break
		}
	}
	if got.Error != "" {
		return StoredValue{}, &ValueError{Msg: got.Error}
	}
	return s.writeValue(ctx, o, name, typ, got.Value, formula)
}

func (s *Store) writeValue(ctx context.Context, o SecretOwner, name, typ, canonical, formula string) (StoredValue, error) {
	rows, err := s.listStored(ctx, o)
	if err != nil {
		return StoredValue{}, err
	}
	total, old := 0, 0
	exists := false
	for _, row := range rows {
		n := len(row.Name) + len(row.Value) + len(row.Formula)
		total += n
		if row.Name == name {
			exists = true
			old = n
		}
	}
	if !exists && len(rows) >= maxValuesPerOwner {
		return StoredValue{}, &ValueError{Msg: "не больше 32 значений"}
	}
	if total-old+len(name)+len(canonical)+len(formula) > maxValueBytes {
		return StoredValue{}, &ValueError{Msg: "значения больше 8 КБ"}
	}
	col, err := o.column()
	if err != nil {
		return StoredValue{}, err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	owner, args := "?", []any{o.ID}
	conflict := fmt.Sprintf("%[1]s, name) WHERE %[1]s IS NOT NULL", col)
	if o.Kind == SecretOwnerWorkspace {
		owner, args = "1", nil
		conflict = "workspace_id, name) WHERE workspace_scope = 1"
	}
	_, err = s.DB.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO named_value (%[1]s, name, type, value, formula, created_at, updated_at)
		VALUES (%[3]s, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(%[2]s
		DO UPDATE SET type = excluded.type, value = excluded.value, formula = excluded.formula, updated_at = excluded.updated_at`, col, conflict, owner),
		append(args, name, typ, canonical, formula, now, now)...)
	if err != nil {
		return StoredValue{}, err
	}
	return StoredValue{Name: name, Type: typ, Value: canonical, Formula: formula}, nil
}

// DeleteNamedValue removes one name. A missing name is not an error.
func (s *Store) DeleteNamedValue(ctx context.Context, o SecretOwner, name string) error {
	cond, args, err := o.match()
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM named_value WHERE `+cond+` AND name = ?`, append(args, name)...)
	return err
}

func (s *Store) listStored(ctx context.Context, o SecretOwner) ([]StoredValue, error) {
	cond, args, err := o.match()
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT name, type, value, formula FROM named_value WHERE `+cond+` ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredValue{}
	for rows.Next() {
		var v StoredValue
		if err := rows.Scan(&v.Name, &v.Type, &v.Value, &v.Formula); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// NotePrivate reports whether the note is flagged private.
func (s *Store) NotePrivate(ctx context.Context, id int64) (bool, error) {
	var priv int
	err := s.DB.QueryRowContext(ctx, `SELECT is_private FROM note WHERE id = ?`, id).Scan(&priv)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	return priv != 0, nil
}

type valueLink struct {
	Owner   SecretOwner
	Title   string
	Private bool
}

// valueLinks is the inheritance chain, nearest first. A note walks its parents (cycle-safe) and then the workspace.
// Anything else uses the secret chain, reversed, so a step sees itself, then its quest, template, questline, workspace.
func (s *Store) valueLinks(ctx context.Context, o SecretOwner) ([]valueLink, error) {
	switch o.Kind {
	case SecretOwnerNote:
		return s.noteValueLinks(ctx, o.ID)
	case SecretOwnerWorkspace:
		return []valueLink{{Owner: o}}, nil
	default:
		chain, err := s.secretChain(ctx, o)
		if err != nil {
			return nil, err
		}
		links := make([]valueLink, len(chain))
		for i := range chain {
			owner := chain[len(chain)-1-i]
			links[i] = valueLink{Owner: owner, Title: s.ownerTitle(ctx, owner)}
		}
		return links, nil
	}
}

func (s *Store) noteValueLinks(ctx context.Context, id int64) ([]valueLink, error) {
	var links []valueLink
	seen := map[int64]bool{}
	for range maxValueChain {
		if id <= 0 || seen[id] {
			break
		}
		var parent sql.NullInt64
		var title string
		var priv int
		err := s.DB.QueryRowContext(ctx, `SELECT parent_id, title, is_private FROM note WHERE id = ?`, id).Scan(&parent, &title, &priv)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return nil, err
		}
		seen[id] = true
		links = append(links, valueLink{Owner: SecretOwner{Kind: SecretOwnerNote, ID: id}, Title: title, Private: priv != 0})
		if !parent.Valid {
			break
		}
		id = parent.Int64
	}
	if len(links) == 0 {
		return nil, ErrNotFound
	}
	return append(links, valueLink{Owner: workspaceOwner(ctx)}), nil
}

func (s *Store) ownerTitle(ctx context.Context, o SecretOwner) string {
	var q string
	switch o.Kind {
	case SecretOwnerNote:
		q = `SELECT title FROM note WHERE id = ?`
	case SecretOwnerQuest:
		q = `SELECT title FROM quest WHERE id = ?`
	case SecretOwnerStep:
		q = `SELECT title FROM queststep WHERE id = ?`
	case SecretOwnerTemplate:
		q = `SELECT title FROM questtemplate WHERE id = ?`
	case SecretOwnerQuestline:
		q = `SELECT title FROM questline WHERE id = ?`
	default:
		return ""
	}
	var title string
	_ = s.DB.QueryRowContext(ctx, q, o.ID).Scan(&title)
	return title
}

// ResolveNamedValues walks the chain and computes formulas in that scope. skipPrivate leaves out notes flagged
// private (MCP reading a public child).
func (s *Store) ResolveNamedValues(ctx context.Context, o SecretOwner, skipPrivate bool) (ValueSet, error) {
	set, err := s.collectNamedValues(ctx, o, skipPrivate)
	if err != nil {
		return ValueSet{}, err
	}
	return evalSet(set), nil
}

func (s *Store) collectNamedValues(ctx context.Context, o SecretOwner, skipPrivate bool) (ValueSet, error) {
	links, err := s.valueLinks(ctx, o)
	if err != nil {
		return ValueSet{}, err
	}
	var set ValueSet
	seen := map[string]bool{}
	for i, link := range links {
		if skipPrivate && link.Private {
			continue
		}
		rows, err := s.listStored(ctx, link.Owner)
		if err != nil {
			return ValueSet{}, err
		}
		if i == 0 && link.Owner == o {
			set.Own = rows
		}
		for _, row := range rows {
			if seen[row.Name] {
				continue
			}
			if len(set.Effective) >= maxEffective {
				set.Truncated = true
				return finishSet(set), nil
			}
			seen[row.Name] = true
			set.Effective = append(set.Effective, ValuePlace{StoredValue: row, From: link.Owner, Title: link.Title})
		}
	}
	return finishSet(set), nil
}

func evalSet(set ValueSet) ValueSet {
	rows := make([]StoredValue, len(set.Effective))
	for i, p := range set.Effective {
		rows[i] = p.StoredValue
	}
	rows = EvalValues(rows)
	by := map[string]StoredValue{}
	for i, row := range rows {
		set.Effective[i].StoredValue = row
		by[row.Name] = row
	}
	for i, row := range set.Own {
		if got, ok := by[row.Name]; ok {
			set.Own[i].Value = got.Value
			set.Own[i].Error = got.Error
		}
	}
	return set
}

func finishSet(set ValueSet) ValueSet {
	if set.Own == nil {
		set.Own = []StoredValue{}
	}
	if set.Effective == nil {
		set.Effective = []ValuePlace{}
	}
	return set
}
