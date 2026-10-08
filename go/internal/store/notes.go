package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/valirum/quests/go/internal/timeutil"
)

type Note struct {
	ID          int64
	Title       string
	Description string
	Pinned      bool
	IsReadme    bool
	IsPrivate   bool
	IsCategory  bool
	SortOrder   int
	ParentID    *int64
	Color       string
	Icon        string
	CustomIcon  *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type NoteRead map[string]any

type NoteFilter struct {
	ParentID    *int64
	ParentUnset bool
	Pinned      *bool
}

const noteCols = `id, title, description, pinned, is_readme, is_private, is_category, sort_order, parent_id, color, icon, custom_icon, created_at, updated_at`

func (s *Store) ListNotes(ctx context.Context, f NoteFilter) ([]Note, error) {
	q := `SELECT ` + noteCols + ` FROM note WHERE 1=1`
	args := []any{}
	if f.ParentUnset {
		q += ` AND parent_id IS NULL`
	} else if f.ParentID != nil {
		q += ` AND parent_id = ?`
		args = append(args, *f.ParentID)
	}
	if f.Pinned != nil {
		v := 0
		if *f.Pinned {
			v = 1
		}
		q += ` AND pinned = ?`
		args = append(args, v)
	}
	q += ` ORDER BY pinned DESC, sort_order, id`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Note, 0)
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) GetNote(ctx context.Context, id int64) (Note, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+noteCols+` FROM note WHERE id = ?`, id)
	n, err := scanNote(row)
	if err == sql.ErrNoRows {
		return Note{}, ErrNotFound
	}
	return n, err
}

func (s *Store) NoteExists(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM note WHERE id = ?`, id).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CreateNote(ctx context.Context, n Note) (Note, error) {
	now := timeutil.NowUTC()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = now
	}
	pin, readme, private, category := 0, 0, 0, 0
	if n.Pinned {
		pin = 1
	}
	if n.IsReadme {
		readme = 1
	}
	if n.IsPrivate {
		private = 1
	}
	if n.IsCategory {
		category = 1
	}
	if n.ParentID != nil && (n.Color == "" || n.Icon == "") {
		parent, err := s.GetNote(ctx, *n.ParentID)
		if err == nil {
			if n.Color == "" {
				n.Color = parent.Color
			}
			if n.Icon == "" {
				n.Icon = parent.Icon
			}
		}
	}
	if n.Color == "" {
		n.Color = "#9a9a9a"
	}
	if n.Icon == "" {
		n.Icon = "document"
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO note (title, description, pinned, is_readme, is_private, is_category, sort_order, parent_id, color, icon, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Title, n.Description, pin, readme, private, category, n.SortOrder, n.ParentID, n.Color, n.Icon,
		timeutil.ToDBUTC(n.CreatedAt), timeutil.ToDBUTC(n.UpdatedAt))
	if err != nil {
		return Note{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Note{}, err
	}
	return s.GetNote(ctx, id)
}

func (s *Store) UpdateNote(ctx context.Context, id int64, patch map[string]any) (Note, error) {
	cur, err := s.GetNote(ctx, id)
	if err != nil {
		return Note{}, err
	}
	if v, ok := patch["title"].(string); ok {
		cur.Title = v
	}
	if v, ok := patch["description"].(string); ok {
		cur.Description = v
	}
	if v, ok := patch["pinned"].(bool); ok {
		cur.Pinned = v
	}
	if v, ok := patch["is_readme"].(bool); ok {
		cur.IsReadme = v
	}
	if v, ok := patch["is_private"].(bool); ok {
		cur.IsPrivate = v
	}
	if v, ok := patch["is_category"].(bool); ok {
		cur.IsCategory = v
	}
	if v, ok := noteAsInt(patch["sort_order"]); ok {
		cur.SortOrder = v
	}
	if _, ok := patch["parent_id"]; ok {
		cur.ParentID = noteAsInt64Ptr(patch["parent_id"])
	}
	if v, ok := patch["color"].(string); ok {
		cur.Color = v
	}
	if v, ok := patch["icon"].(string); ok {
		cur.Icon = v
	}
	if cur.ParentID != nil && *cur.ParentID == id {
		return Note{}, ErrNoteCycle
	}
	if cur.ParentID != nil {
		ok, err := s.noteWouldCycle(ctx, id, *cur.ParentID)
		if err != nil {
			return Note{}, err
		}
		if ok {
			return Note{}, ErrNoteCycle
		}
	}
	pin, readme, private, category := 0, 0, 0, 0
	if cur.Pinned {
		pin = 1
	}
	if cur.IsReadme {
		readme = 1
	}
	if cur.IsPrivate {
		private = 1
	}
	if cur.IsCategory {
		category = 1
	}
	if cur.Color == "" {
		cur.Color = "#9a9a9a"
	}
	if cur.Icon == "" {
		cur.Icon = "document"
	}
	now := timeutil.NowUTC()
	_, err = s.DB.ExecContext(ctx, `
		UPDATE note SET title=?, description=?, pinned=?, is_readme=?, is_private=?, is_category=?, sort_order=?, parent_id=?, color=?, icon=?, updated_at=?
		WHERE id=?`,
		cur.Title, cur.Description, pin, readme, private, category, cur.SortOrder, cur.ParentID, cur.Color, cur.Icon,
		timeutil.ToDBUTC(now), id)
	if err != nil {
		return Note{}, err
	}
	return s.GetNote(ctx, id)
}

var ErrNoteCycle = errors.New("parent would create a cycle")

func (s *Store) noteWouldCycle(ctx context.Context, noteID, parentID int64) (bool, error) {
	cur := parentID
	for i := 0; i < 64; i++ {
		if cur == noteID {
			return true, nil
		}
		n, err := s.GetNote(ctx, cur)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if n.ParentID == nil {
			return false, nil
		}
		cur = *n.ParentID
	}
	return true, nil
}

func (s *Store) SetNoteIcon(ctx context.Context, id int64, filename string) (Note, error) {
	now := timeutil.NowUTC()
	_, err := s.DB.ExecContext(ctx, `
		UPDATE note SET custom_icon=?, updated_at=? WHERE id=?`, filename, timeutil.ToDBUTC(now), id)
	if err != nil {
		return Note{}, err
	}
	return s.GetNote(ctx, id)
}

func (s *Store) ClearNoteIcon(ctx context.Context, id int64, dataDir string) (Note, error) {
	cur, err := s.GetNote(ctx, id)
	if err != nil {
		return Note{}, err
	}
	now := timeutil.NowUTC()
	_, err = s.DB.ExecContext(ctx, `UPDATE note SET custom_icon=NULL, updated_at=? WHERE id=?`, timeutil.ToDBUTC(now), id)
	if err != nil {
		return Note{}, err
	}
	if cur.CustomIcon != nil && *cur.CustomIcon != "" {
		_ = os.Remove(filepath.Join(dataDir, "note-icons", *cur.CustomIcon))
	}
	return s.GetNote(ctx, id)
}

// CopyNoteCustomIcon duplicates parent's uploaded icon file under the child id.
func (s *Store) CopyNoteCustomIcon(ctx context.Context, fromID, toID int64, dataDir string) (Note, error) {
	from, err := s.GetNote(ctx, fromID)
	if err != nil {
		return Note{}, err
	}
	if from.CustomIcon == nil || *from.CustomIcon == "" || !SafeIconName(*from.CustomIcon) {
		return s.GetNote(ctx, toID)
	}
	src := filepath.Join(dataDir, "note-icons", *from.CustomIcon)
	ext := filepath.Ext(*from.CustomIcon)
	if ext == "" {
		ext = ".png"
	}
	name := fmt.Sprintf("%d%s", toID, ext)
	dst := filepath.Join(dataDir, "note-icons", name)
	raw, err := os.ReadFile(src)
	if err != nil {
		return s.GetNote(ctx, toID)
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return Note{}, err
	}
	return s.SetNoteIcon(ctx, toID, name)
}

func (s *Store) DeleteNote(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE note SET parent_id=NULL WHERE parent_id=?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM note WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func scanNote(row rowScanner) (Note, error) {
	var n Note
	var pin, readme, private, category int
	var parent sql.NullInt64
	var created, updated sql.NullString
	var colorNS, iconNS, custom sql.NullString
	err := row.Scan(&n.ID, &n.Title, &n.Description, &pin, &readme, &private, &category, &n.SortOrder, &parent, &colorNS, &iconNS, &custom, &created, &updated)
	if err != nil {
		return n, err
	}
	n.Pinned = pin != 0
	n.IsReadme = readme != 0
	n.IsPrivate = private != 0
	n.IsCategory = category != 0
	if parent.Valid {
		v := parent.Int64
		n.ParentID = &v
	}
	if colorNS.Valid && colorNS.String != "" {
		n.Color = colorNS.String
	} else {
		n.Color = "#9a9a9a"
	}
	if iconNS.Valid && iconNS.String != "" {
		n.Icon = iconNS.String
	} else {
		n.Icon = "document"
	}
	if custom.Valid && custom.String != "" {
		s := custom.String
		n.CustomIcon = &s
	}
	if t, err := timeutil.ParseFlexible(created.String); err == nil {
		n.CreatedAt = t
	}
	if t, err := timeutil.ParseFlexible(updated.String); err == nil {
		n.UpdatedAt = t
	}
	return n, nil
}

func NoteToRead(n Note) NoteRead {
	out := NoteRead{
		"id":          n.ID,
		"title":       n.Title,
		"description": n.Description,
		"pinned":      n.Pinned,
		"is_readme":   n.IsReadme,
		"is_private":  n.IsPrivate,
		"is_category": n.IsCategory,
		"sort_order":  n.SortOrder,
		"parent_id":   n.ParentID,
		"color":       n.Color,
		"icon":        n.Icon,
		"custom_icon": nil,
		"icon_url":    nil,
		"created_at":  timeutil.ToUTCISO(&n.CreatedAt),
		"updated_at":  timeutil.ToUTCISO(&n.UpdatedAt),
	}
	if n.CustomIcon != nil && *n.CustomIcon != "" {
		out["custom_icon"] = *n.CustomIcon
		u := fmt.Sprintf("/api/notes/%d/icon", n.ID)
		if iso := timeutil.ToUTCISO(&n.UpdatedAt); iso != nil {
			u += "?v=" + *iso
		}
		out["icon_url"] = u
	}
	return out
}

func NoteBrief(n Note) NoteRead {
	out := NoteRead{
		"id":          n.ID,
		"title":       n.Title,
		"parent_id":   n.ParentID,
		"pinned":      n.Pinned,
		"is_readme":   n.IsReadme,
		"is_private":  n.IsPrivate,
		"is_category": n.IsCategory,
		"color":       n.Color,
		"icon":        n.Icon,
	}
	if n.CustomIcon != nil && *n.CustomIcon != "" {
		out["icon_url"] = fmt.Sprintf("/api/notes/%d/icon", n.ID)
	}
	return out
}

func noteAsInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	default:
		return 0, false
	}
}

func noteAsInt64Ptr(v any) *int64 {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case float64:
		n := int64(t)
		return &n
	case int64:
		return &t
	case int:
		n := int64(t)
		return &n
	default:
		return nil
	}
}

type LinkDoc struct {
	Kind  string
	ID    int64
	Title string
	Body  string
}

func (s *Store) ListLinkCorpus(ctx context.Context) ([]LinkDoc, error) {
	out := make([]LinkDoc, 0)
	add := func(q string) error {
		rows, err := s.DB.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d LinkDoc
			var desc string
			if err := rows.Scan(&d.Kind, &d.ID, &d.Title, &desc); err != nil {
				return err
			}
			d.Body = d.Title + "\n" + desc
			out = append(out, d)
		}
		return rows.Err()
	}
	if err := add(`SELECT 'note', id, title, description FROM note`); err != nil {
		return nil, err
	}
	if err := add(`SELECT 'quest', id, title, description FROM quest`); err != nil {
		return nil, err
	}
	if err := add(`SELECT 'questline', id, title, description FROM questline`); err != nil {
		return nil, err
	}
	if err := add(`SELECT 'step', id, title, description FROM queststep`); err != nil {
		return nil, err
	}
	return out, nil
}
