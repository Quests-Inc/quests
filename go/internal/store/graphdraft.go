package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/timeutil"
)

// DraftOp is one pending edit made on the relations map. Nothing it describes
// exists in the real tables until the draft is committed.
type DraftOp struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload" tstype:"any"`
	CreatedAt time.Time       `json:"created_at"`
}

// TmpID is the placeholder id an op's created entity answers to inside the
// draft, e.g. "t12" for the entity made by op 12.
func TmpID(opID int64) string { return "t" + itoa(opID) }

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// ListDraftOps returns the pending operations in the order they were made.
func (s *Store) ListDraftOps(ctx context.Context) ([]DraftOp, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, kind, payload, created_at FROM graphdraft_op ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DraftOp{}
	for rows.Next() {
		var o DraftOp
		var payload, at string
		if err := rows.Scan(&o.ID, &o.Kind, &payload, &at); err != nil {
			return nil, err
		}
		o.Payload = json.RawMessage(payload)
		o.CreatedAt, _ = timeutil.ParseFlexible(at)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) AddDraftOp(ctx context.Context, kind string, payload json.RawMessage) (DraftOp, error) {
	now := timeutil.NowUTC()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO graphdraft_op (kind, payload, created_at) VALUES (?, ?, ?)`,
		kind, string(payload), timeutil.ToDBUTC(now))
	if err != nil {
		return DraftOp{}, err
	}
	id, _ := res.LastInsertId()
	return DraftOp{ID: id, Kind: kind, Payload: payload, CreatedAt: now}, nil
}

// DeleteDraftOps removes ops by id.
func (s *Store) DeleteDraftOps(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM graphdraft_op WHERE id IN (`+ph+`)`, args...)
	return err
}

// UpdateDraftOpPayload replaces an op's payload (used to swap placeholder ids
// for real ones after a partial commit).
func (s *Store) UpdateDraftOpPayload(ctx context.Context, id int64, payload json.RawMessage) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE graphdraft_op SET payload = ? WHERE id = ?`, string(payload), id)
	return err
}

// DraftRoots returns the extra map roots kept with the draft, as raw JSON.
func (s *Store) DraftRoots(ctx context.Context) (json.RawMessage, error) {
	var roots string
	err := s.DB.QueryRowContext(ctx, `SELECT roots FROM graphdraft_state ORDER BY id LIMIT 1`).Scan(&roots)
	if err != nil {
		return json.RawMessage("[]"), nil //nolint:nilerr // no row yet = no roots
	}
	return json.RawMessage(roots), nil
}

func (s *Store) SetDraftRoots(ctx context.Context, roots json.RawMessage) error {
	// One row per workspace (row-level security scopes the UPDATE); insert on first use.
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	res, err := s.DB.ExecContext(ctx, `UPDATE graphdraft_state SET roots = ?, updated_at = ?`, string(roots), now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO graphdraft_state (roots, updated_at) VALUES (?, ?)`, string(roots), now)
	return err
}

// ClearDraft drops every pending op (the map's roots are view state and stay);
// the removed ops are returned so staged files can be cleaned up.
func (s *Store) ClearDraft(ctx context.Context) ([]DraftOp, error) {
	ops, err := s.ListDraftOps(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM graphdraft_op`); err != nil {
		return nil, err
	}
	return ops, nil
}

// SetAttachmentOwner re-parents an attachment. The WebDAV path is untouched:
// bytes are found through the stored path, not through the owner.
func (s *Store) SetAttachmentOwner(ctx context.Context, id int64, ownerType string, ownerID int64) (Attachment, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE attachment SET owner_type = ?, owner_id = ? WHERE id = ?`, ownerType, ownerID, id)
	if err != nil {
		return Attachment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Attachment{}, ErrNotFound
	}
	return s.GetAttachment(ctx, id)
}
