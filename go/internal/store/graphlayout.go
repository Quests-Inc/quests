package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// GraphLayout is a saved view of the relations map: its State is the map's own JSON (centre, depth, filters, open links,
// card positions, camera), which the server keeps without reading. A layout is its author's, inside one workspace.
type GraphLayout struct {
	ID        int64           `json:"id"`
	Title     string          `json:"title"`
	State     json.RawMessage `json:"state,omitempty" tstype:"any"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// layoutOwner is the acting user; work without one (a loopback server with sign-in off) shares the empty owner.
func layoutOwner(ctx context.Context) any {
	if id := wsctx.UserFrom(ctx); id > 0 {
		return id
	}
	return nil
}

const layoutMine = `user_id IS NOT DISTINCT FROM CAST(? AS INTEGER)`

// ListGraphLayouts returns the user's layouts, without their state, newest change first.
func (s *Store) ListGraphLayouts(ctx context.Context) ([]GraphLayout, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, title, created_at, updated_at FROM graph_layout WHERE `+layoutMine+` ORDER BY updated_at DESC, id DESC`, layoutOwner(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GraphLayout{}
	for rows.Next() {
		var l GraphLayout
		var created, updated string
		if err := rows.Scan(&l.ID, &l.Title, &created, &updated); err != nil {
			return nil, err
		}
		l.CreatedAt, _ = timeutil.ParseFlexible(created)
		l.UpdatedAt, _ = timeutil.ParseFlexible(updated)
		out = append(out, l)
	}
	return out, rows.Err()
}

// CountGraphLayouts is how many layouts the user has.
func (s *Store) CountGraphLayouts(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_layout WHERE `+layoutMine, layoutOwner(ctx)).Scan(&n)
	return n, err
}

// GetGraphLayout returns one of the user's layouts with its state.
func (s *Store) GetGraphLayout(ctx context.Context, id int64) (GraphLayout, error) {
	var l GraphLayout
	var state, created, updated string
	err := s.DB.QueryRowContext(ctx, `SELECT id, title, state, created_at, updated_at FROM graph_layout WHERE id = ? AND `+layoutMine, id, layoutOwner(ctx)).
		Scan(&l.ID, &l.Title, &state, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return GraphLayout{}, ErrNotFound
	}
	if err != nil {
		return GraphLayout{}, err
	}
	l.State = json.RawMessage(state)
	l.CreatedAt, _ = timeutil.ParseFlexible(created)
	l.UpdatedAt, _ = timeutil.ParseFlexible(updated)
	return l, nil
}

// CreateGraphLayout saves a new layout for the user.
func (s *Store) CreateGraphLayout(ctx context.Context, title string, state json.RawMessage) (GraphLayout, error) {
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	var id int64
	if err := s.DB.QueryRowContext(ctx, `INSERT INTO graph_layout (user_id, title, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		layoutOwner(ctx), title, string(state), now, now).Scan(&id); err != nil {
		return GraphLayout{}, err
	}
	return s.GetGraphLayout(ctx, id)
}

// UpdateGraphLayout renames a layout and/or replaces its state (nil leaves a field as it is).
func (s *Store) UpdateGraphLayout(ctx context.Context, id int64, title *string, state json.RawMessage) (GraphLayout, error) {
	cur, err := s.GetGraphLayout(ctx, id)
	if err != nil {
		return GraphLayout{}, err
	}
	if title != nil {
		cur.Title = *title
	}
	if state != nil {
		cur.State = state
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	if _, err := s.DB.ExecContext(ctx, `UPDATE graph_layout SET title = ?, state = ?, updated_at = ? WHERE id = ? AND `+layoutMine,
		cur.Title, string(cur.State), now, id, layoutOwner(ctx)); err != nil {
		return GraphLayout{}, err
	}
	return s.GetGraphLayout(ctx, id)
}

// DeleteGraphLayout removes one of the user's layouts.
func (s *Store) DeleteGraphLayout(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM graph_layout WHERE id = ? AND `+layoutMine, id, layoutOwner(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
