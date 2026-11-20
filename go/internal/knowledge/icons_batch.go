package knowledge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// IconBatchError rejects the whole batch. Nothing is written.
type IconBatchError struct {
	Code int
	Msg  string
}

func (e *IconBatchError) Error() string { return e.Msg }

// NoteIconBatch sets one color and one built-in icon on many notes, and gives each its own copy of one image.
// The image is either the uploaded bytes or the file already stored for CopyFrom.
type NoteIconBatch struct {
	IDs      []int64
	Color    string
	Icon     string
	CopyFrom int64
	File     []byte
	Ext      string
}

var (
	noteIconColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	noteIconName  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

const noteIconBatchMax = 200

// ApplyNoteIcons updates every id or none. A missing id, a sample, or a foreign workspace (not visible here) fails the call.
func (s *Store) ApplyNoteIcons(ctx context.Context, dataDir string, b NoteIconBatch) ([]Note, error) {
	if len(b.IDs) == 0 {
		return nil, &IconBatchError{422, "ids required"}
	}
	if len(b.IDs) > noteIconBatchMax {
		return nil, &IconBatchError{422, "too many notes"}
	}
	seen := map[int64]bool{}
	for _, id := range b.IDs {
		if id <= 0 || seen[id] {
			return nil, &IconBatchError{422, "ids must be unique positive note ids"}
		}
		seen[id] = true
	}
	if b.Color == "" && b.Icon == "" && b.CopyFrom == 0 && len(b.File) == 0 {
		return nil, &IconBatchError{422, "color, icon, file or copy_from required"}
	}
	if b.Color != "" && !noteIconColor.MatchString(b.Color) {
		return nil, &IconBatchError{422, "color must be #rgb or #rrggbb"}
	}
	if b.Icon != "" && !noteIconName.MatchString(b.Icon) {
		return nil, &IconBatchError{422, "unknown icon name"}
	}
	if b.CopyFrom != 0 && len(b.File) > 0 {
		return nil, &IconBatchError{422, "pass a file or copy_from, not both"}
	}
	if len(b.File) > 512*1024 {
		return nil, &IconBatchError{422, "icon too large or empty"}
	}
	notes := make([]Note, 0, len(b.IDs))
	var missing []int64
	for _, id := range b.IDs {
		n, err := s.GetNote(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				missing = append(missing, id)
				continue
			}
			return nil, err
		}
		if n.IsSample {
			return nil, &IconBatchError{403, fmt.Sprintf("note %d is a sample", id)}
		}
		notes = append(notes, n)
	}
	if len(missing) > 0 {
		return nil, &IconBatchError{404, fmt.Sprintf("unknown note ids: %s", joinIDs(missing))}
	}
	raw, ext := b.File, b.Ext
	if b.CopyFrom != 0 {
		src, err := s.GetNote(ctx, b.CopyFrom)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, &IconBatchError{404, "copy_from note not found"}
			}
			return nil, err
		}
		if src.CustomIcon == nil || *src.CustomIcon == "" || !SafeIconName(*src.CustomIcon) {
			return nil, &IconBatchError{422, "copy_from has no uploaded icon"}
		}
		raw, err = os.ReadFile(filepath.Join(dataDir, "note-icons", *src.CustomIcon))
		if err != nil {
			return nil, &IconBatchError{422, "copy_from icon file is missing"}
		}
		ext = filepath.Ext(*src.CustomIcon)
	}
	if len(raw) > 0 && ext == "" {
		ext = ".png"
	}
	now := timeutil.NowUTC()
	out := make([]Note, 0, len(notes))
	for _, n := range notes {
		filename := ""
		if len(raw) > 0 {
			name, err := writeNoteIconFile(dataDir, n.ID, ext, raw)
			if err != nil {
				return nil, err
			}
			if n.CustomIcon != nil && *n.CustomIcon != "" && *n.CustomIcon != name {
				_ = os.Remove(filepath.Join(dataDir, "note-icons", *n.CustomIcon))
			}
			filename = name
		}
		updated, err := s.patchNoteAppearance(ctx, n.ID, b.Color, b.Icon, filename, now)
		if err != nil {
			return nil, err
		}
		out = append(out, updated)
	}
	return out, nil
}

func (s *Store) patchNoteAppearance(ctx context.Context, id int64, color, icon, filename string, now time.Time) (Note, error) {
	sets := []string{"updated_at=?"}
	args := []any{timeutil.ToDBUTC(now)}
	if color != "" {
		sets = append(sets, "color=?")
		args = append(args, color)
	}
	if icon != "" {
		sets = append(sets, "icon=?")
		args = append(args, icon)
	}
	if filename != "" {
		sets = append(sets, "custom_icon=?")
		args = append(args, filename)
	}
	args = append(args, id)
	q := "UPDATE note SET " + strings.Join(sets, ", ") + " WHERE id=?"
	if _, err := s.DB.ExecContext(ctx, q, args...); err != nil {
		return Note{}, err
	}
	return s.GetNote(ctx, id)
}

func writeNoteIconFile(dataDir string, id int64, ext string, raw []byte) (string, error) {
	if ext == "" {
		ext = ".png"
	}
	if ext[0] != '.' {
		ext = "." + ext
	}
	name := strconv.FormatInt(id, 10) + ext
	if !SafeIconName(name) {
		return "", &IconBatchError{422, "bad icon file name"}
	}
	dir := filepath.Join(dataDir, "note-icons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		return "", err
	}
	return name, nil
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ", ")
}
