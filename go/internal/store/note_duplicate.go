package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/limits"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// DuplicateNote copies content and own values atomically. Checks, secrets and folder/pin flags are deliberately absent.
// Attachment references keep their original workspace-local ids; original attachments must remain available.
func (s *Store) DuplicateNote(ctx context.Context, source int64, title *string, parent *int64, parentSet, hidePrivate bool, dataDir string) (knowledge.Note, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return knowledge.Note{}, err
	}
	defer tx.Rollback()

	var id int64
	var n knowledge.Note
	var private int
	if err := tx.QueryRowContext(ctx, `SELECT id,title,description,parent_id,color,icon,is_private,custom_icon FROM note WHERE id=? FOR UPDATE`, source).Scan(&n.ID, &n.Title, &n.Description, &n.ParentID, &n.Color, &n.Icon, &private, &n.CustomIcon); err != nil {
		return knowledge.Note{}, err
	}
	n.IsPrivate = private != 0
	if hidePrivate && n.IsPrivate {
		return knowledge.Note{}, execution.ErrPrivateValue
	}
	if title == nil {
		n.Title += " (копия)"
	} else {
		n.Title = strings.TrimSpace(*title)
	}
	if n.Title == "" {
		return knowledge.Note{}, fmt.Errorf("нужно название")
	}
	if parentSet {
		n.ParentID = parent
	}
	if n.ParentID != nil {

		var private int
		if err := tx.QueryRowContext(ctx, "SELECT is_private FROM note WHERE id=?", *n.ParentID).Scan(&private); err != nil {
			return knowledge.Note{}, err
		}
		if hidePrivate && private != 0 {
			return knowledge.Note{}, execution.ErrPrivateValue
		}
	}
	var count, size int64
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(name)+octet_length(value)+octet_length(formula)+octet_length(roll_arg)),0) FROM named_value WHERE note_id=?`, source).Scan(&count, &size); err != nil {
		return knowledge.Note{}, err
	}
	if count > limits.Get("values_own") || size > limits.Get("values_bytes") {
		return knowledge.Note{}, fmt.Errorf("переменные копии превышают текущую квоту")
	}
	nowTime := timeutil.NowUTC()
	now := timeutil.ToDBUTC(nowTime)
	result, err := tx.ExecContext(ctx, `INSERT INTO note (title,description,parent_id,color,icon,is_private,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`, n.Title, n.Description, n.ParentID, n.Color, n.Icon, boolInt(n.IsPrivate), now, now)
	if err != nil {
		return knowledge.Note{}, err
	}
	id, err = result.LastInsertId()
	if err != nil {
		return knowledge.Note{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO named_value (note_id,name,type,value,formula,roll_script,roll_arg,created_at,updated_at) SELECT ?,name,type,value,formula,roll_script,roll_arg,?,? FROM named_value WHERE note_id=?`, id, now, now, source)
	if err != nil {
		return knowledge.Note{}, err
	}
	if err := s.Execution.CheckCopiedValueBudget(ctx, tx, execution.SecretOwner{Kind: execution.SecretOwnerNote, ID: id}, hidePrivate); err != nil {
		return knowledge.Note{}, err
	}
	var iconPath string
	committed := false
	defer func() {
		if !committed && iconPath != "" {
			_ = os.Remove(iconPath)
		}
	}()
	if n.CustomIcon != nil && *n.CustomIcon != "" {
		if !knowledge.SafeIconName(*n.CustomIcon) {
			return knowledge.Note{}, fmt.Errorf("неверное имя значка")
		}
		raw, err := os.ReadFile(filepath.Join(dataDir, "note-icons", *n.CustomIcon))
		if err != nil {
			return knowledge.Note{}, err
		}
		name := strconv.FormatInt(id, 10) + filepath.Ext(*n.CustomIcon)
		iconPath = filepath.Join(dataDir, "note-icons", name)
		if err := os.WriteFile(iconPath, raw, 0600); err != nil {
			return knowledge.Note{}, err
		}
		n.CustomIcon = &name
		if _, err := tx.ExecContext(ctx, "UPDATE note SET custom_icon=? WHERE id=?", name, id); err != nil {
			return knowledge.Note{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return knowledge.Note{}, err
	}
	committed = true
	n.ID = id
	n.Pinned = false
	n.IsCategory = false
	n.IsReadme = false
	n.IsSample = false
	n.SortOrder = 0
	n.CreatedAt = nowTime
	n.UpdatedAt = nowTime
	return n, nil
}
