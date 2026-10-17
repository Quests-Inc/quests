package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/hooks"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

const (
	MaxHookName        = 200
	MaxHookTimeout     = 600
	DefaultHookTimeout = 30
)

// Hook is an event trigger bound to a library script (quest=438, ADR-0003).
type Hook struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Enabled    bool      `json:"enabled"`
	Events     []string  `json:"events"`
	EventsRaw  []string  `json:"events_raw"`
	ScriptID   int64     `json:"script_id"`
	QuestID    *int64    `json:"quest_id"`
	RunOn      string    `json:"run_on"`
	TimeoutSec int       `json:"timeout_sec"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// ScriptTitle is filled on list/get for the UI.
	ScriptTitle string `json:"script_title,omitempty"`
	NeedsRunner bool   `json:"needs_runner,omitempty"`
}

// HookIn is what create and update accept.
type HookIn struct {
	Name       *string
	Enabled    *bool
	Events     []string // required on create
	ScriptID   *int64
	QuestID    *int64 // nil = leave; pointer to 0 / use ClearQuest
	ClearQuest bool
	RunOn      *string
	TimeoutSec *int
}

const hookCols = `h.id, h.name, h.enabled, h.events, h.events_raw, h.script_id, h.quest_id, h.run_on, h.timeout_sec, h.created_at, h.updated_at`

func scanHook(sc interface{ Scan(...any) error }) (Hook, error) {
	var h Hook
	var enabled int
	var events, raw string
	var quest sql.NullInt64
	var created, updated string
	if err := sc.Scan(&h.ID, &h.Name, &enabled, &events, &raw, &h.ScriptID, &quest, &h.RunOn, &h.TimeoutSec, &created, &updated); err != nil {
		return Hook{}, err
	}
	h.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(events), &h.Events)
	if h.Events == nil {
		h.Events = []string{}
	}
	_ = json.Unmarshal([]byte(raw), &h.EventsRaw)
	if h.EventsRaw == nil {
		h.EventsRaw = []string{}
	}
	if quest.Valid {
		v := quest.Int64
		h.QuestID = &v
	}
	h.CreatedAt, _ = timeutil.ParseFlexible(created)
	h.UpdatedAt, _ = timeutil.ParseFlexible(updated)
	return h, nil
}

func encodeJSONStrings(ss []string) string {
	if ss == nil {
		ss = []string{}
	}
	b, _ := json.Marshal(ss)
	return string(b)
}

func (s *Store) attachScriptMeta(ctx context.Context, list []Hook) error {
	if len(list) == 0 {
		return nil
	}
	for i := range list {
		var title string
		var needs int
		err := s.DB.QueryRowContext(ctx, `SELECT title, needs_runner FROM script WHERE id = ?`, list[i].ScriptID).Scan(&title, &needs)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		list[i].ScriptTitle = title
		list[i].NeedsRunner = needs == 1
	}
	return nil
}

// ListHooks returns every hook of the workspace, newest first.
func (s *Store) ListHooks(ctx context.Context) ([]Hook, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+hookCols+` FROM hook h ORDER BY h.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hook{}
	for rows.Next() {
		h, err := scanHook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachScriptMeta(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetHook returns one hook.
func (s *Store) GetHook(ctx context.Context, id int64) (Hook, error) {
	h, err := scanHook(s.DB.QueryRowContext(ctx, `SELECT `+hookCols+` FROM hook h WHERE h.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Hook{}, ErrNotFound
	}
	if err != nil {
		return Hook{}, err
	}
	list := []Hook{h}
	if err := s.attachScriptMeta(ctx, list); err != nil {
		return Hook{}, err
	}
	return list[0], nil
}

// HooksForEvent returns enabled hooks that match kind and optional quest (workspace already scoped by RLS).
func (s *Store) HooksForEvent(ctx context.Context, kind string, questID *int64) ([]Hook, error) {
	list, err := s.ListHooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Hook, 0, len(list))
	for _, h := range list {
		if !h.Enabled {
			continue
		}
		ok := false
		for _, e := range h.Events {
			if e == kind {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		if h.QuestID != nil {
			if questID == nil || *h.QuestID != *questID {
				continue
			}
		}
		out = append(out, h)
	}
	return out, nil
}

func (s *Store) prepareHook(ctx context.Context, events []string, scriptID int64, runOn string, timeout int) (expanded, raw []string, runOnOut string, timeoutOut int, err error) {
	raw = make([]string, 0, len(events))
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e != "" {
			raw = append(raw, e)
		}
	}
	expanded = hooks.ExpandEvents(raw)
	if len(expanded) == 0 {
		return nil, nil, "", 0, errBad("нужен хотя бы один event")
	}
	sc, err := s.GetScript(ctx, scriptID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil, "", 0, errBad("скрипт не найден")
	}
	if err != nil {
		return nil, nil, "", 0, err
	}
	on := kernel.NormalizeRunOn(&runOn)
	runOnOut = ""
	if on != nil {
		runOnOut = *on
	}
	if !kernel.ValidRunOn(on) {
		return nil, nil, "", 0, errBad("run_on: " + kernel.ErrBadRunOn.Error())
	}
	if sc.NeedsRunner && runOnOut == "" {
		return nil, nil, "", 0, kernel.ErrNoRunner
	}
	timeoutOut = timeout
	if timeoutOut <= 0 {
		timeoutOut = DefaultHookTimeout
	}
	if timeoutOut > MaxHookTimeout {
		return nil, nil, "", 0, errBad("timeout_sec: максимум 600")
	}
	return expanded, raw, runOnOut, timeoutOut, nil
}

func (s *Store) questExists(ctx context.Context, id int64) (bool, error) {
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM quest WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CreateHook stores a new hook.
func (s *Store) CreateHook(ctx context.Context, name string, events []string, scriptID int64, questID *int64, runOn string, timeout int, enabled bool) (Hook, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > MaxHookName {
		return Hook{}, errBad("name: слишком длинное")
	}
	expanded, raw, runOnOut, timeoutOut, err := s.prepareHook(ctx, events, scriptID, runOn, timeout)
	if err != nil {
		return Hook{}, err
	}
	if questID != nil {
		ok, err := s.questExists(ctx, *questID)
		if err != nil {
			return Hook{}, err
		}
		if !ok {
			return Hook{}, errBad("квест не найден")
		}
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	en := 0
	if enabled {
		en = 1
	}
	var quest any
	if questID != nil {
		quest = *questID
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO hook (name, enabled, events, events_raw, script_id, quest_id, run_on, timeout_sec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, en, encodeJSONStrings(expanded), encodeJSONStrings(raw), scriptID, quest, runOnOut, timeoutOut, now, now)
	if err != nil {
		return Hook{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Hook{}, err
	}
	return s.GetHook(ctx, id)
}

// UpdateHook changes the given fields of a hook.
func (s *Store) UpdateHook(ctx context.Context, id int64, in HookIn) (Hook, error) {
	cur, err := s.GetHook(ctx, id)
	if err != nil {
		return Hook{}, err
	}
	name, enabled, scriptID, runOn, timeout := cur.Name, cur.Enabled, cur.ScriptID, cur.RunOn, cur.TimeoutSec
	events := cur.EventsRaw
	if len(events) == 0 {
		events = cur.Events
	}
	questID := cur.QuestID
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if len([]rune(name)) > MaxHookName {
			return Hook{}, errBad("name: слишком длинное")
		}
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.Events != nil {
		events = in.Events
	}
	if in.ScriptID != nil {
		scriptID = *in.ScriptID
	}
	if in.RunOn != nil {
		runOn = *in.RunOn
	}
	if in.TimeoutSec != nil {
		timeout = *in.TimeoutSec
	}
	if in.ClearQuest {
		questID = nil
	} else if in.QuestID != nil {
		if *in.QuestID == 0 {
			questID = nil
		} else {
			questID = in.QuestID
		}
	}
	expanded, raw, runOnOut, timeoutOut, err := s.prepareHook(ctx, events, scriptID, runOn, timeout)
	if err != nil {
		return Hook{}, err
	}
	if questID != nil {
		ok, err := s.questExists(ctx, *questID)
		if err != nil {
			return Hook{}, err
		}
		if !ok {
			return Hook{}, errBad("квест не найден")
		}
	}
	en := 0
	if enabled {
		en = 1
	}
	var quest any
	if questID != nil {
		quest = *questID
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	res, err := s.DB.ExecContext(ctx, `UPDATE hook SET name = ?, enabled = ?, events = ?, events_raw = ?, script_id = ?, quest_id = ?, run_on = ?, timeout_sec = ?, updated_at = ?
		WHERE id = ?`, name, en, encodeJSONStrings(expanded), encodeJSONStrings(raw), scriptID, quest, runOnOut, timeoutOut, now, id)
	if err != nil {
		return Hook{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Hook{}, ErrNotFound
	}
	return s.GetHook(ctx, id)
}

// DeleteHook removes a hook.
func (s *Store) DeleteHook(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM hook WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// HookEventsCatalog is the aliases and kinds the UI/API expose (same as CLI).
func HookEventsCatalog() map[string]any {
	aliases := hooks.EventAliases()
	seen := map[string]struct{}{}
	kinds := []string{}
	for _, targets := range aliases {
		for _, k := range targets {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			kinds = append(kinds, k)
		}
	}
	// Stable extras that aliases may not cover alone.
	for _, k := range []string{"quest_appeared", "quest_started", "step_progress", "quest_expired"} {
		if _, ok := seen[k]; !ok {
			kinds = append(kinds, k)
		}
	}
	return map[string]any{"aliases": aliases, "kinds": kinds}
}
