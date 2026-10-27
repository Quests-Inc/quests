package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// RemoveStarter trashes untouched examples, preserving edited items and owners of user-added or edited children.
func (s *Store) RemoveStarter(ctx context.Context) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(?))", fmt.Sprintf("starter:%d", wsctx.From(ctx))); err != nil {
		return 0, err
	}
	type item struct {
		table   string
		id      int64
		applied string
		keep    bool
	}
	rows, err := tx.QueryContext(ctx, `SELECT entity_kind,entity_id,applied_hash FROM seed_item WHERE removed=0`)
	if err != nil {
		return 0, err
	}
	items := map[string]*item{}
	for rows.Next() {
		i := &item{}
		if err := rows.Scan(&i.table, &i.id, &i.applied); err != nil {
			rows.Close()
			return 0, err
		}
		items[seedID(i.table, i.id)] = i
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	p := &seedPlan{tx: tx}
	for _, i := range items {
		hash, err := p.entityHash(ctx, &seedEntry{table: i.table, id: i.id})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		i.keep = err != nil || hash != i.applied
	}
	// A new child belongs to the user. Its seeded parent must remain live.
	for _, spec := range ExportTables {
		if !spec.HasID {
			continue
		}
		parents := map[string]string{}
		for col, table := range spec.FK {
			parents[col] = table
		}
		if spec.SelfFK != "" {
			parents[spec.SelfFK] = spec.Name
		}
		for col, parent := range parents {
			for _, i := range items {
				if i.table != parent || i.keep {
					continue
				}
				var exists bool
				q := "SELECT EXISTS(SELECT 1 FROM " + spec.Name + " c WHERE c." + col + "=? AND NOT EXISTS(SELECT 1 FROM seed_item si WHERE si.entity_kind=? AND si.entity_id=c.id))"
				if err := tx.QueryRowContext(ctx, q, i.id, spec.Name).Scan(&exists); err != nil {
					return 0, err
				}
				if exists {
					i.keep = true
				}
			}
		}
	}
	// Changes to a seeded child protect its owners too (e.g. a completed tutorial step or an edited variable).
	for changed := true; changed; {
		changed = false
		for _, spec := range ExportTables {
			parents := map[string]string{}
			for col, table := range spec.FK {
				parents[col] = table
			}
			if spec.SelfFK != "" {
				parents[spec.SelfFK] = spec.Name
			}
			for _, i := range items {
				if i.table != spec.Name || !i.keep {
					continue
				}
				for col, parent := range parents {
					var id sql.NullInt64
					err := tx.QueryRowContext(ctx, "SELECT "+col+" FROM "+spec.Name+" WHERE id=?", i.id).Scan(&id)
					if errors.Is(err, sql.ErrNoRows) {
						continue
					}
					if err != nil {
						return 0, err
					}
					if target := items[seedID(parent, id.Int64)]; id.Valid && target != nil && !target.keep {
						target.keep = true
						changed = true
					}
				}
			}
		}
	}
	// Open the trash policy for the updates. Plain reads above intentionally excluded already-deleted examples.
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.trash','1',true)`); err != nil {
		return 0, err
	}
	count := 0
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	var actor any
	if id := wsctx.UserFrom(ctx); id > 0 {
		actor = id
	}
	for _, i := range items {
		if i.keep {
			continue
		}
		switch i.table {
		case "note", "quest", "script", "hook", "questline", "questtemplate":
			if i.table == "note" {
				if _, err := tx.ExecContext(ctx, `UPDATE note SET deleted_children=ARRAY(SELECT id FROM note c WHERE c.parent_id=? AND c.deleted_at IS NULL ORDER BY id) WHERE id=?`, i.id, i.id); err != nil {
					return 0, err
				}
			}
			r, err := tx.ExecContext(ctx, "UPDATE "+i.table+" SET deleted_at=?,deleted_by=? WHERE id=? AND deleted_at IS NULL", now, actor, i.id)
			if err != nil {
				return 0, err
			}
			n, _ := r.RowsAffected()
			count += int(n)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE seed_item SET removed=1 WHERE removed=0`); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}
