package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/refs"
)

// Workspace export / import (quest=349).
//
// A workspace is exported table by table (JSON lines) and imported into a NEW,
// empty workspace. Primary keys are global serials, so every imported row gets a
// fresh id; foreign keys are remapped through per-table old→new maps, and the
// kind=ID references embedded in text are rewritten with the same maps (refs.Rewrite).

// ExportFormat identifies the archive; ExportVersion is bumped on incompatible changes.
const (
	ExportFormat  = "quests-workspace"
	ExportVersion = 1
)

type polyFK struct {
	TypeCol, IDCol string
}

// TableSpec describes how one table travels. Tables are listed in insertion order.
type TableSpec struct {
	Name   string
	HasID  bool
	FK     map[string]string // column → referenced table
	Poly   []polyFK          // (type column, id column) pairs naming their target table by type
	SelfFK string            // column referencing the same table, fixed up in a second pass
	Text   []string          // columns scanned for kind=ID references
	// Secret tables are exported only on request.
	Secret bool
}

// Kind names in kind=ID tokens and entity_type / owner_type columns → table.
var kindTable = map[string]string{
	"quest": "quest", "questline": "questline", "note": "note", "step": "queststep",
	"attachment": "attachment", "template": "questtemplate", "tag": "tag", "category": "questcategory",
}

var ExportTables = []TableSpec{
	{Name: "questcategory", HasID: true},
	{Name: "questline", HasID: true, FK: map[string]string{"category_id": "questcategory"}, Text: []string{"title", "description"}},
	{Name: "questtemplate", HasID: true, FK: map[string]string{"category_id": "questcategory", "questline_id": "questline"}, Text: []string{"title", "description"}},
	{Name: "questtemplatestep", HasID: true, FK: map[string]string{"template_id": "questtemplate"}, Text: []string{"title", "description", "check_command"}},
	{Name: "quest", HasID: true, FK: map[string]string{"category_id": "questcategory", "questline_id": "questline", "template_id": "questtemplate"}, Text: []string{"title", "description", "source"}},
	{Name: "queststep", HasID: true, FK: map[string]string{"quest_id": "quest"}, Text: []string{"title", "description", "check_command"}},
	{Name: "tag", HasID: true},
	{Name: "quest_tag", FK: map[string]string{"quest_id": "quest", "tag_id": "tag"}},
	{Name: "template_tag", FK: map[string]string{"template_id": "questtemplate", "tag_id": "tag"}},
	{Name: "note", HasID: true, SelfFK: "parent_id", Text: []string{"title", "description"}},
	{Name: "attachment", HasID: true, Poly: []polyFK{{"owner_type", "owner_id"}}, Text: []string{"comment"}},
	{Name: "attachment_revision", HasID: true, FK: map[string]string{"attachment_id": "attachment"}, Text: []string{"comment"}},
	{Name: "templateemitroll", HasID: true, FK: map[string]string{"template_id": "questtemplate"}, Text: []string{"picked_refs"}},
	{Name: "questchangelog", HasID: true, FK: map[string]string{"quest_id": "quest"}, Poly: []polyFK{{"entity_type", "entity_id"}, {"parent_type", "parent_id"}}, Text: []string{"title", "detail", "comment"}},
	{Name: "secret", HasID: true, FK: map[string]string{"questline_id": "questline", "template_id": "questtemplate", "quest_id": "quest", "step_id": "queststep", "note_id": "note"}, Secret: true},
}

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// ReadTable streams the rows of a workspace table (under the context's workspace)
// as column→value maps ready for JSON.
func (s *Store) ReadTable(ctx context.Context, spec TableSpec, fn func(row map[string]any) error) error {
	q := `SELECT * FROM ` + spec.Name
	if spec.HasID {
		q += ` ORDER BY id`
	}
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		row := make(map[string]any, len(cols))
		skip := false
		for i, c := range cols {
			if c == "workspace_id" || c == "search_tsv" { // search_tsv is generated: rebuilt on import
				continue
			}
			if spec.Name == "secret" && c == "value" {
				// Archives carry the value itself (sealed values would only open on this server).
				if str, ok := vals[i].(string); ok {
					plain, okOpen := s.Automation().OpenSecret(str)
					if !okOpen {
						skip = true // not readable under this key: leave the row out
						break
					}
					row[c] = plain
					continue
				}
			}
			switch v := vals[i].(type) {
			case []byte:
				row[c] = string(v)
			case time.Time:
				row[c] = v.UTC().Format("2006-01-02 15:04:05.999999")
			default:
				row[c] = v
			}
		}
		if skip {
			continue
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ErrAttachmentMissing is returned by ImportOptions.Attachment when the archive has no
// bytes for a file (exports skip files the file server no longer has): the row is skipped.
var ErrAttachmentMissing = errors.New("attachment bytes missing from the archive")

// ImportOptions control one import.
type ImportOptions struct {
	// Secrets imports the secret table when the archive has it.
	Secrets bool
	// Attachment returns the new WebDAV path for an old one, having stored the bytes
	// (scanned) under it. ownerType/ownerID/filename are the already-remapped owner.
	// nil = the archive carries no file bytes: attachment rows are skipped.
	Attachment func(ctx context.Context, oldPath, ownerType string, ownerID int64, filename string) (newPath string, err error)
	// MaxRowsPerTable caps untrusted input (0 = 200000).
	MaxRowsPerTable int
}

// ImportReport is what an import did.
type ImportReport struct {
	Counts          map[string]int `json:"counts"`
	StrippedCommand int            `json:"stripped_commands"`
	// DanglingRefs counts kind=ID references to entities missing from the archive.
	DanglingRefs int            `json:"dangling_refs"`
	Skipped      map[string]int `json:"skipped,omitempty"`
	// IDs maps table → old id → new id (for the caller's own follow-ups, e.g. icon files).
	IDs map[string]map[int64]int64 `json:"-"`
}

// TableRows yields the archived rows of one table.
type TableRows func(table string, fn func(row map[string]any) error) error

var checkProgressRe = regexp.MustCompile(`\bquests progress (\d+)\b`)

// ImportWorkspace loads an archive into the (new, empty) workspace on ctx, in one
// transaction. Nothing is visible, and nothing stays, if it fails.
func (s *Store) ImportWorkspace(ctx context.Context, tables TableRows, opt ImportOptions) (rep ImportReport, err error) {
	if wsctx.From(ctx) <= 0 {
		return rep, errors.New("import needs a target workspace")
	}
	if opt.MaxRowsPerTable <= 0 {
		opt.MaxRowsPerTable = 200000
	}
	rep.Counts = map[string]int{}
	rep.Skipped = map[string]int{}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	ids := map[string]map[int64]int64{}
	for _, t := range ExportTables {
		ids[t.Name] = map[int64]int64{}
	}
	allowed := map[string]map[string]bool{}
	columnsOf := func(table string) (map[string]bool, error) {
		if c, ok := allowed[table]; ok {
			return c, nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ?`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		c := map[string]bool{}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return nil, err
			}
			c[n] = true
		}
		allowed[table] = c
		return c, rows.Err()
	}
	type owner struct {
		kind string
		id   int64
	}
	attachOwner := map[int64]owner{} // new attachment id → its remapped owner
	attachPaths := map[string]string{}
	noteParents := map[int64]int64{} // new note id → OLD parent id

	mapID := func(table string, old any) (int64, bool) {
		n, ok := toInt64(old)
		if !ok {
			return 0, false
		}
		nid, ok := ids[table][n]
		return nid, ok
	}

	for _, spec := range ExportTables {
		if spec.Secret && !opt.Secrets {
			continue
		}
		cols, err := columnsOf(spec.Name)
		if err != nil {
			return rep, err
		}
		n := 0
		err = tables(spec.Name, func(row map[string]any) error {
			n++
			if n > opt.MaxRowsPerTable {
				return fmt.Errorf("%s: more than %d rows", spec.Name, opt.MaxRowsPerTable)
			}
			oldID, _ := toInt64(row["id"])
			vals := map[string]any{}
			for c, v := range row {
				if c == "id" || c == "workspace_id" || c == "search_tsv" {
					continue
				}
				if !identRe.MatchString(c) || !cols[c] {
					return fmt.Errorf("%s: unknown column %q", spec.Name, c)
				}
				vals[c] = v
			}
			// Foreign keys. A reference to something absent from the archive is dropped when
			// the column is nullable, and the row is skipped when it is required.
			for col, target := range spec.FK {
				v, present := vals[col]
				if !present || v == nil {
					continue
				}
				nid, ok := mapID(target, v)
				if !ok {
					if _, nullable := nullableFK[spec.Name+"."+col]; nullable {
						vals[col] = nil
						continue
					}
					rep.Skipped[spec.Name]++
					return nil
				}
				vals[col] = nid
			}
			for _, p := range spec.Poly {
				t, _ := vals[p.TypeCol].(string)
				v := vals[p.IDCol]
				if t == "" || v == nil {
					continue
				}
				target, known := kindTable[t]
				nid, ok := int64(0), false
				if known {
					nid, ok = mapID(target, v)
				}
				if !ok {
					if spec.Name == "attachment" { // owner is required
						rep.Skipped[spec.Name]++
						return nil
					}
					vals[p.IDCol] = nil
					continue
				}
				vals[p.IDCol] = nid
			}
			if spec.SelfFK != "" {
				vals[spec.SelfFK] = nil // linked in a second pass, once every row has its new id
			}
			// Commands with no runner cannot run anywhere now: they are not imported.
			switch spec.Name {
			case "queststep", "questtemplatestep":
				if c, _ := vals["check_command"].(string); strings.TrimSpace(c) != "" && isEmptyStr(vals["run_on"]) {
					vals["check_command"], vals["check_interval_seconds"] = nil, nil
					rep.StrippedCommand++
				}
			case "questtemplate":
				if c, _ := vals["emit_pool_command"].(string); strings.TrimSpace(c) != "" && isEmptyStr(vals["emit_run_on"]) {
					vals["emit_pool_command"] = nil
					rep.StrippedCommand++
				}
			case "note":
				if c, _ := vals["check_command"].(string); strings.TrimSpace(c) != "" && isEmptyStr(vals["run_on"]) {
					vals["check_command"], vals["check_interval_seconds"] = nil, nil
					rep.StrippedCommand++
				}
			}

			// Attachments: bring the bytes along and point the row at the new path.
			if spec.Name == "attachment" || spec.Name == "attachment_revision" {
				if opt.Attachment == nil {
					rep.Skipped[spec.Name]++
					return nil
				}
				oldPath, _ := vals["webdav_path"].(string)
				var ot string
				var oid int64
				if spec.Name == "attachment" {
					ot, _ = vals["owner_type"].(string)
					oid, _ = toInt64(vals["owner_id"])
				} else {
					aid, _ := toInt64(vals["attachment_id"])
					o, ok := attachOwner[aid]
					if !ok {
						rep.Skipped[spec.Name]++
						return nil
					}
					ot, oid = o.kind, o.id
				}
				np, ok := attachPaths[oldPath]
				if !ok {
					fn, _ := vals["filename"].(string)
					var aerr error
					np, aerr = opt.Attachment(ctx, oldPath, ot, oid, fn)
					if errors.Is(aerr, ErrAttachmentMissing) {
						rep.Skipped[spec.Name]++
						return nil
					}
					if aerr != nil {
						return aerr
					}
					attachPaths[oldPath] = np
				}
				vals["webdav_path"] = np
			}
			if spec.Name == "questline" || spec.Name == "note" {
				vals["custom_icon"] = nil // icon files are re-attached by the caller
			}

			if spec.Name == "secret" {
				if str, ok := vals["value"].(string); ok {
					sealed, err := s.Automation().SealSecret(str)
					if err != nil {
						return err
					}
					vals["value"] = sealed
				}
			}
			names := make([]string, 0, len(vals))
			args := make([]any, 0, len(vals))
			for c, v := range vals {
				names = append(names, c)
				args = append(args, v)
			}
			q := "INSERT INTO " + spec.Name + " (" + strings.Join(names, ", ") + ") VALUES (" + strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ") + ")"
			res, err := tx.ExecContext(ctx, q, args...)
			if err != nil {
				return fmt.Errorf("%s: %w", spec.Name, err)
			}
			if spec.HasID {
				newID, _ := res.LastInsertId()
				ids[spec.Name][oldID] = newID
				switch spec.Name {
				case "attachment":
					ot, _ := vals["owner_type"].(string)
					oid, _ := toInt64(vals["owner_id"])
					attachOwner[newID] = owner{ot, oid}
				case "note":
					if p, ok := toInt64(row[spec.SelfFK]); ok {
						noteParents[newID] = p
					}
				}
			}
			rep.Counts[spec.Name]++
			return nil
		})
		if err != nil {
			return rep, err
		}
	}
	// Notes form a tree: parents are linked now that every note has its new id.
	for newID, oldParent := range noteParents {
		if np, ok := ids["note"][oldParent]; ok {
			if _, err = tx.ExecContext(ctx, `UPDATE note SET parent_id = ? WHERE id = ?`, np, newID); err != nil {
				return rep, err
			}
		}
	}

	// References inside text follow the entities to their new ids.
	// A reference to something that is not in the archive is defused ("quest#232"): left as
	// "quest=232" it could silently start pointing at an unrelated entity that happens to
	// carry that id in the new workspace.
	resolve := func(kind string, id int64) (string, bool) {
		t, ok := kindTable[kind]
		if !ok {
			return "", false
		}
		if nid, ok := ids[t][id]; ok {
			return kind + "=" + strconv.FormatInt(nid, 10), true
		}
		rep.DanglingRefs++
		return kind + "#" + strconv.FormatInt(id, 10), true
	}
	for _, spec := range ExportTables {
		if len(spec.Text) == 0 || (spec.Secret && !opt.Secrets) {
			continue
		}
		sel := "SELECT id, " + strings.Join(spec.Text, ", ") + " FROM " + spec.Name
		rows, qerr := tx.QueryContext(ctx, sel)
		if qerr != nil {
			return rep, qerr
		}
		type upd struct {
			id   int64
			vals []any
		}
		var todo []upd
		for rows.Next() {
			var id int64
			cur := make([]sql.NullString, len(spec.Text))
			ptrs := []any{&id}
			for i := range cur {
				ptrs = append(ptrs, &cur[i])
			}
			if err = rows.Scan(ptrs...); err != nil {
				rows.Close()
				return rep, err
			}
			changed := false
			out := make([]any, len(cur))
			for i, c := range cur {
				if !c.Valid {
					out[i] = nil
					continue
				}
				nv := refs.Rewrite(c.String, resolve)
				if spec.Name == "queststep" || spec.Name == "questtemplatestep" {
					if spec.Text[i] == "check_command" {
						nv = checkProgressRe.ReplaceAllStringFunc(nv, func(m string) string {
							num, _ := strconv.ParseInt(checkProgressRe.FindStringSubmatch(m)[1], 10, 64)
							if nid, ok := ids["quest"][num]; ok {
								return "quests progress " + strconv.FormatInt(nid, 10)
							}
							return m
						})
					}
				}
				if nv != c.String {
					changed = true
				}
				out[i] = nv
			}
			if changed {
				todo = append(todo, upd{id, out})
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return rep, err
		}
		for _, u := range todo {
			set := make([]string, len(spec.Text))
			for i, c := range spec.Text {
				set[i] = c + " = ?"
			}
			args := append(append([]any{}, u.vals...), u.id)
			if _, err = tx.ExecContext(ctx, "UPDATE "+spec.Name+" SET "+strings.Join(set, ", ")+" WHERE id = ?", args...); err != nil {
				return rep, err
			}
		}
	}
	// quest.source is "template:N" for emitted quests.
	srows, qerr := tx.QueryContext(ctx, `SELECT id, source FROM quest WHERE source LIKE 'template:%'`)
	if qerr != nil {
		return rep, qerr
	}
	type src struct {
		id  int64
		val string
	}
	var srcs []src
	for srows.Next() {
		var id int64
		var v string
		if err = srows.Scan(&id, &v); err != nil {
			srows.Close()
			return rep, err
		}
		if old, perr := strconv.ParseInt(strings.TrimPrefix(v, "template:"), 10, 64); perr == nil {
			if nid, ok := ids["questtemplate"][old]; ok {
				srcs = append(srcs, src{id, "template:" + strconv.FormatInt(nid, 10)})
			}
		}
	}
	srows.Close()
	for _, u := range srcs {
		if _, err = tx.ExecContext(ctx, `UPDATE quest SET source = ? WHERE id = ?`, u.val, u.id); err != nil {
			return rep, err
		}
	}
	rep.IDs = ids
	if err = tx.Commit(); err != nil {
		return rep, err
	}
	return rep, nil
}

var nullableFK = map[string]struct{}{
	"questline.category_id": {}, "questtemplate.category_id": {}, "questtemplate.questline_id": {},
	"quest.category_id": {}, "quest.questline_id": {}, "quest.template_id": {},
	"questchangelog.quest_id": {}, "secret.questline_id": {}, "secret.template_id": {},
	"secret.quest_id": {}, "secret.step_id": {}, "secret.note_id": {},
}

func isEmptyStr(v any) bool {
	s, _ := v.(string)
	return strings.TrimSpace(s) == ""
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

var _ = timeutil.NowUTC
