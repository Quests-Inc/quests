package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

type Questline struct {
	ID            int64
	Title         string
	Description   string
	CategoryID    *int64
	Color         string
	Icon          string
	CustomIcon    *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CategorySlug  *string
	CategoryLabel *string
	CategoryColor *string
}

// QuestlineRead is a questline as the API sends it.
type QuestlineRead struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	CategoryID    *int64  `json:"category_id"`
	Color         string  `json:"color"`
	Icon          string  `json:"icon"`
	CustomIcon    *string `json:"custom_icon"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	CategorySlug  *string `json:"category_slug"`
	CategoryLabel *string `json:"category_label"`
	CategoryColor *string `json:"category_color"`
	IconURL       *string `json:"icon_url"`
	// Refs is filled for a single-questline fetch only: the mentions in the title and description.
	Refs any `json:"refs,omitempty"`
}

func (s *Store) ListQuestlines(ctx context.Context) ([]QuestlineRead, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT l.id, l.title, l.description, l.category_id, l.color, l.icon, l.custom_icon,
			l.created_at, l.updated_at, c.slug, c.label, c.color
		FROM questline l
		LEFT JOIN questcategory c ON c.id = l.category_id
		ORDER BY l.created_at, l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]QuestlineRead, 0)
	for rows.Next() {
		ql, err := scanQuestline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, questlineToRead(ql))
	}
	return out, rows.Err()
}

func (s *Store) GetQuestline(ctx context.Context, id int64) (QuestlineRead, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT l.id, l.title, l.description, l.category_id, l.color, l.icon, l.custom_icon,
			l.created_at, l.updated_at, c.slug, c.label, c.color
		FROM questline l
		LEFT JOIN questcategory c ON c.id = l.category_id
		WHERE l.id = ?`, id)
	ql, err := scanQuestline(row)
	if err == sql.ErrNoRows {
		return QuestlineRead{}, ErrNotFound
	}
	if err != nil {
		return QuestlineRead{}, err
	}
	return questlineToRead(ql), nil
}

func (s *Store) CreateQuestline(ctx context.Context, title, description string, categoryID *int64, color, icon string) (QuestlineRead, error) {
	now := timeutil.NowUTC()
	if color == "" {
		color = "#9a9a9a"
	}
	if icon == "" {
		icon = "document"
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO questline (title, description, category_id, color, icon, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		title, description, nullI64(categoryID), color, icon, timeutil.ToDBUTC(now), timeutil.ToDBUTC(now))
	if err != nil {
		return QuestlineRead{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetQuestline(ctx, id)
}

func (s *Store) UpdateQuestline(ctx context.Context, id int64, fields map[string]any) (QuestlineRead, error) {
	cur, err := s.GetQuestline(ctx, id)
	if err != nil {
		return QuestlineRead{}, err
	}
	title, desc, color, icon, catID := cur.Title, cur.Description, cur.Color, cur.Icon, cur.CategoryID
	if v, ok := fields["title"].(string); ok {
		title = v
	}
	if v, ok := fields["description"].(string); ok {
		desc = v
	}
	if v, ok := fields["color"].(string); ok {
		color = v
	}
	if v, ok := fields["icon"].(string); ok {
		icon = v
	}
	if raw, ok := fields["category_id"]; ok {
		switch v := raw.(type) {
		case nil:
			catID = nil
		case float64:
			i := int64(v)
			catID = &i
		case int64:
			catID = &v
		}
	}
	now := timeutil.NowUTC()
	_, err = s.DB.ExecContext(ctx, `
		UPDATE questline SET title=?, description=?, category_id=?, color=?, icon=?, updated_at=?
		WHERE id=?`, title, desc, nullI64(catID), color, icon, timeutil.ToDBUTC(now), id)
	if err != nil {
		return QuestlineRead{}, err
	}
	if catID != nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE quest SET category_id=? WHERE questline_id=?`, *catID, id)
		_, _ = s.DB.ExecContext(ctx, `UPDATE questtemplate SET category_id=? WHERE questline_id=?`, *catID, id)
	}
	return s.GetQuestline(ctx, id)
}

func (s *Store) DeleteQuestline(ctx context.Context, id int64, dataDir string) error {
	ql, err := s.GetQuestline(ctx, id)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE quest SET questline_id=NULL WHERE questline_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE questtemplate SET questline_id=NULL WHERE questline_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM questline WHERE id=?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if ql.CustomIcon != nil && *ql.CustomIcon != "" {
		_ = os.Remove(filepath.Join(dataDir, "questline-icons", *ql.CustomIcon))
	}
	return nil
}

func (s *Store) SetQuestlineIcon(ctx context.Context, id int64, filename string) (QuestlineRead, error) {
	now := timeutil.NowUTC()
	_, err := s.DB.ExecContext(ctx, `
		UPDATE questline SET custom_icon=?, updated_at=? WHERE id=?`, filename, timeutil.ToDBUTC(now), id)
	if err != nil {
		return QuestlineRead{}, err
	}
	return s.GetQuestline(ctx, id)
}

func (s *Store) ClearQuestlineIcon(ctx context.Context, id int64, dataDir string) (QuestlineRead, error) {
	ql, err := s.GetQuestline(ctx, id)
	if err != nil {
		return QuestlineRead{}, err
	}
	now := timeutil.NowUTC()
	_, err = s.DB.ExecContext(ctx, `UPDATE questline SET custom_icon=NULL, updated_at=? WHERE id=?`, timeutil.ToDBUTC(now), id)
	if err != nil {
		return QuestlineRead{}, err
	}
	if ql.CustomIcon != nil && *ql.CustomIcon != "" {
		_ = os.Remove(filepath.Join(dataDir, "questline-icons", *ql.CustomIcon))
	}
	return s.GetQuestline(ctx, id)
}

func scanQuestline(row rowScanner) (Questline, error) {
	var q Questline
	var catID sql.NullInt64
	var custom, created, updated sql.NullString
	var cSlug, cLabel, cColor sql.NullString
	err := row.Scan(&q.ID, &q.Title, &q.Description, &catID, &q.Color, &q.Icon, &custom,
		&created, &updated, &cSlug, &cLabel, &cColor)
	if err != nil {
		return q, err
	}
	if catID.Valid {
		v := catID.Int64
		q.CategoryID = &v
	}
	if custom.Valid && custom.String != "" {
		s := custom.String
		q.CustomIcon = &s
	}
	if created.Valid {
		t, _ := timeutil.ParseFlexible(created.String)
		q.CreatedAt = t
	}
	if updated.Valid {
		t, _ := timeutil.ParseFlexible(updated.String)
		q.UpdatedAt = t
	}
	if cSlug.Valid {
		s := cSlug.String
		q.CategorySlug = &s
	}
	if cLabel.Valid {
		s := cLabel.String
		q.CategoryLabel = &s
	}
	if cColor.Valid {
		s := cColor.String
		q.CategoryColor = &s
	}
	if q.Color == "" {
		q.Color = "#9a9a9a"
	}
	if q.Icon == "" {
		q.Icon = "document"
	}
	return q, nil
}

func questlineToRead(q Questline) QuestlineRead {
	out := QuestlineRead{
		ID: q.ID, Title: q.Title, Description: q.Description, CategoryID: q.CategoryID,
		Color: q.Color, Icon: q.Icon,
		CreatedAt: derefISO(&q.CreatedAt), UpdatedAt: derefISO(&q.UpdatedAt),
		CategorySlug: q.CategorySlug, CategoryLabel: q.CategoryLabel, CategoryColor: q.CategoryColor,
	}
	if q.CustomIcon != nil && *q.CustomIcon != "" {
		icon := *q.CustomIcon
		out.CustomIcon = &icon
		u := fmt.Sprintf("/api/questlines/%d/icon", q.ID)
		if iso := timeutil.ToUTCISO(&q.UpdatedAt); iso != nil {
			u += "?v=" + *iso
		}
		out.IconURL = &u
	}
	return out
}

func derefISO(t *time.Time) string {
	if iso := timeutil.ToUTCISO(t); iso != nil {
		return *iso
	}
	return ""
}

func SafeIconName(name string) bool {
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
