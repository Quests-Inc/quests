package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

//go:embed starter/ru.json
var starterJSON []byte

type starterSet struct {
	Version int                         `json:"version"`
	Tables  map[string][]map[string]any `json:"tables"`
}
type seedEntry struct {
	key, table, content string
	old, id             int64
	write, known        bool
	source              map[string]any
}
type seedPlan struct {
	tx       *sql.Tx
	version  int
	entries  map[string]*seedEntry
	ids      map[string]map[int64]int64
	versions map[int64]int
	report   *SeedReport
}
type SeedReport struct {
	Version   int `json:"version"`
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Preserved int `json:"preserved"`
	Deleted   int `json:"deleted"`
}

func seedID(table string, id int64) string { return fmt.Sprintf("%s:%d", table, id) }
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func seedFields(row map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range row {
		if k != "_key" && k != "id" && k != "created_at" && k != "updated_at" {
			out[k] = v
		}
	}
	return out
}

// SeedStarter adds new examples and updates only untouched examples; removed or edited entries are remembered.
func (s *Store) SeedStarter(ctx context.Context) (SeedReport, error) {
	return s.seedStarter(ctx, starterJSON)
}
func (s *Store) seedStarter(ctx context.Context, data []byte) (report SeedReport, err error) {
	var set starterSet
	if err = json.Unmarshal(data, &set); err != nil {
		return report, err
	}
	report.Version = set.Version
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(?))", fmt.Sprintf("starter:%d", wsctx.From(ctx))); err != nil {
		return report, err
	}
	p := &seedPlan{tx: tx, version: set.Version, entries: map[string]*seedEntry{}, ids: map[string]map[int64]int64{}, versions: map[int64]int{}, report: &report}
	for _, spec := range ExportTables {
		p.ids[spec.Name] = map[int64]int64{}
	}
	for table, rows := range set.Tables {
		if _, ok := p.ids[table]; !ok {
			return report, fmt.Errorf("starter: unknown table %s", table)
		}
		for _, row := range rows {
			if table == "script_version" {
				continue
			}
			old, _ := toInt64(row["id"])
			key, _ := row["_key"].(string)
			if old <= 0 || key == "" {
				return report, fmt.Errorf("starter: key and id required for %s", table)
			}
			content := seedFields(row)
			if table == "script" {
				for _, v := range set.Tables["script_version"] {
					sid, _ := toInt64(v["script_id"])
					if sid == old {
						content["graph"] = v["graph"]
					}
				}
			}
			e := &seedEntry{key: key, table: table, old: old, source: row, content: digest(content), write: true}
			var oldContent, applied string
			var removed int
			err = tx.QueryRowContext(ctx, `SELECT entity_id,content_hash,applied_hash,removed FROM seed_item WHERE key=?`, key).Scan(&e.id, &oldContent, &applied, &removed)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return report, err
			}
			e.known = err == nil
			if e.known {
				current, hashErr := p.entityHash(ctx, e)
				if removed != 0 || errors.Is(hashErr, sql.ErrNoRows) {
					e.write = false
					report.Deleted++
				} else if hashErr != nil {
					return report, hashErr
				} else {
					p.ids[table][old] = e.id
					e.write = current == applied && e.content != oldContent
					if !e.write {
						report.Preserved++
					}
				}
			}
			if table == "script" && e.known && e.write {
				var ver int
				if err = tx.QueryRowContext(ctx, `SELECT version FROM script WHERE id=?`, e.id).Scan(&ver); err != nil {
					return report, err
				}
				p.versions[old] = ver + 1
			}
			p.entries[seedID(table, old)] = e
		}
	}
	tables := func(table string, fn func(map[string]any) error) error {
		for _, row := range set.Tables[table] {
			out := map[string]any{}
			for k, v := range row {
				if k != "_key" {
					out[k] = v
				}
			}
			if err := fn(out); err != nil {
				return err
			}
		}
		return nil
	}
	_, err = s.ImportWorkspace(ctx, tables, ImportOptions{seed: p})
	return report, err
}
func (p *seedPlan) prepare(table string, old int64, row map[string]any) bool {
	if table == "script_version" {
		sid, _ := toInt64(row["script_id"])
		e := p.entries[seedID("script", sid)]
		if e == nil || !e.write {
			return false
		}
		if n := p.versions[sid]; n > 0 {
			row["version"] = n
		}
		row["created_at"] = timeutil.ToDBUTC(timeutil.NowUTC())
		return true
	}
	e := p.entries[seedID(table, old)]
	if e == nil || !e.write {
		return false
	}
	if table == "script" && p.versions[old] > 0 {
		row["version"] = p.versions[old]
	}
	if e.known {
		delete(row, "created_at")
	} else if _, ok := row["created_at"]; ok {
		row["created_at"] = timeutil.ToDBUTC(timeutil.NowUTC())
	}
	if _, ok := row["updated_at"]; ok {
		row["updated_at"] = timeutil.ToDBUTC(timeutil.NowUTC())
	}
	return true
}
func (p *seedPlan) entityHash(ctx context.Context, e *seedEntry) (string, error) {
	var raw string
	if err := p.tx.QueryRowContext(ctx, "SELECT row_to_json(t)::text FROM "+e.table+" t WHERE id=?", e.id).Scan(&raw); err != nil {
		return "", err
	}
	var current map[string]any
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return "", err
	}
	fields := map[string]any{}
	for k, v := range current {
		switch k {
		case "id", "workspace_id", "search_tsv", "created_at", "updated_at", "deleted_at", "deleted_by", "deleted_children":
			continue
		}
		fields[k] = v
	}
	if e.table == "script" {
		var graph string
		if err := p.tx.QueryRowContext(ctx, `SELECT graph FROM script_version WHERE script_id=? AND version=?`, e.id, current["version"]).Scan(&graph); err != nil {
			return "", err
		}
		fields["graph"] = graph
	}
	if e.table == "quest" || e.table == "questtemplate" {
		table, column := "quest_tag", "quest_id"
		if e.table == "questtemplate" {
			table, column = "template_tag", "template_id"
		}
		rows, err := p.tx.QueryContext(ctx, "SELECT tag_id FROM "+table+" WHERE "+column+"=? ORDER BY tag_id", e.id)
		if err != nil {
			return "", err
		}
		tags := []int64{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return "", err
			}
			tags = append(tags, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
		fields["tags"] = tags
	}
	return digest(fields), nil
}
func (p *seedPlan) finish(ctx context.Context, ids map[string]map[int64]int64) error {
	for _, e := range p.entries {
		if !e.write {
			continue
		}
		e.id = ids[e.table][e.old]
		if e.id == 0 {
			continue
		}
		applied, err := p.entityHash(ctx, e)
		if err != nil {
			return err
		}
		_, err = p.tx.ExecContext(ctx, `INSERT INTO seed_item (key,entity_kind,entity_id,set_version,content_hash,applied_hash) VALUES (?,?,?,?,?,?) ON CONFLICT(workspace_id,key) DO UPDATE SET entity_id=excluded.entity_id,set_version=excluded.set_version,content_hash=excluded.content_hash,applied_hash=excluded.applied_hash`, e.key, e.table, e.id, p.version, e.content, applied)
		if err != nil {
			return err
		}
		if e.known {
			p.report.Updated++
		} else {
			p.report.Added++
		}
	}
	return nil
}
