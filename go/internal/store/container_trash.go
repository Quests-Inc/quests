package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/quests-inc/quests/go/internal/execution"
)

func containerTable(kind string) (string, error) {
	switch kind {
	case "template":
		return "questtemplate", nil
	case "questline":
		return "questline", nil
	}
	return "", fmt.Errorf("unknown container kind")
}
func (s *Store) containerTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.trash','1',true)`); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

type TrashedContainer struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	DeletedAt string `json:"deleted_at"`
	DeletedBy *int64 `json:"deleted_by"`
	PurgeAt   string `json:"purge_at"`
}

func (s *Store) ListTrashedContainers(ctx context.Context, kind string) ([]TrashedContainer, error) {
	table, err := containerTable(kind)
	if err != nil {
		return nil, err
	}
	tx, err := s.containerTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,title,deleted_at,deleted_by,deleted_at+interval '30 days' FROM "+table+" WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashedContainer{}
	for rows.Next() {
		row := TrashedContainer{Kind: kind}
		if err := rows.Scan(&row.ID, &row.Title, &row.DeletedAt, &row.DeletedBy, &row.PurgeAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
func (s *Store) RestoreContainer(ctx context.Context, kind string, id int64) error {
	table, err := containerTable(kind)
	if err != nil {
		return err
	}
	tx, err := s.containerTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Restored templates are disabled until their owner explicitly enables them.
	extra := ""
	if kind == "template" {
		extra = ",enabled=0"
	}
	r, err := tx.ExecContext(ctx, "UPDATE "+table+" SET deleted_at=NULL,deleted_by=NULL"+extra+" WHERE id=? AND deleted_at IS NOT NULL", id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// PurgeContainer detaches independent records; the trash and ordinary rows must both be visible to FK cleanup.
func (s *Store) PurgeContainer(ctx context.Context, kind string, id int64) error {
	table, err := containerTable(kind)
	if err != nil {
		return err
	}
	tx, err := s.containerTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var one int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM "+table+" WHERE id=? AND deleted_at IS NOT NULL", id).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if kind == "template" {
		if err := execution.CopyTemplateSecretsToLiveQuests(ctx, tx, id); err != nil {
			return err
		}
		for _, q := range []string{`UPDATE quest SET template_id=NULL WHERE template_id=?`, `DELETE FROM script_binding WHERE subject_type='template' AND subject_id=?`, `DELETE FROM templateemitroll WHERE template_id=?`, `DELETE FROM questtemplatestep WHERE template_id=?`} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
	} else {
		for _, q := range []string{`UPDATE quest SET questline_id=NULL WHERE questline_id=?`, `UPDATE questtemplate SET questline_id=NULL WHERE questline_id=?`} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) TrashedContainerIcon(ctx context.Context, kind string, id int64) (string, error) {
	if kind != "questline" {
		return "", nil
	}
	tx, err := s.containerTx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var icon string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(custom_icon,'') FROM questline WHERE id=? AND deleted_at IS NOT NULL`, id).Scan(&icon)
	return icon, err
}
