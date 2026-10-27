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
	// ScopeKind and ScopeID are the area of the hook: `workspace` (ScopeID nil), `quest`, `step`, `note`, or `note_tree` (a note
	// and everything under it). QuestID mirrors a quest area for the old API.
	ScopeKind string `json:"scope_kind"`
	ScopeID   *int64 `json:"scope_id"`
	// CooldownSec is the least time between two runs for the same entity and event; MaxPerHour caps the runs of the hook, and a
	// hook that goes over is switched off (DisabledReason says why). IncludePrivate lets the events of private notes in.
	CooldownSec    int    `json:"cooldown_sec"`
	MaxPerHour     int    `json:"max_per_hour"`
	IncludePrivate bool   `json:"include_private"`
	DisabledReason string `json:"disabled_reason,omitempty"`
	// ScriptTitle is filled on list/get for the UI.
	ScriptTitle string `json:"script_title,omitempty"`
	NeedsRunner bool   `json:"needs_runner,omitempty"`
}

// HookIn is what create and update accept.
type HookIn struct {
	Name    *string
	Enabled *bool
	Events  []string // required on create
	// Area and limits (hooks 2.0). ScopeKind "" leaves the area as it is; ScopeID is ignored for `workspace`.
	ScopeKind      *string
	ScopeID        *int64
	CooldownSec    *int
	MaxPerHour     *int
	IncludePrivate *bool
	ScriptID       *int64
	QuestID        *int64 // nil = leave; pointer to 0 / use ClearQuest
	ClearQuest     bool
	RunOn          *string
	TimeoutSec     *int
}

const hookCols = `h.id, h.name, h.enabled, h.events, h.events_raw, h.script_id, h.quest_id, h.run_on, h.timeout_sec, h.created_at, h.updated_at,
	h.scope_kind, h.scope_id, h.cooldown_sec, h.max_per_hour, h.include_private, h.disabled_reason`

func scanHook(sc interface{ Scan(...any) error }) (Hook, error) {
	var h Hook
	var enabled int
	var events, raw string
	var quest, scopeID sql.NullInt64
	var created, updated string
	var private int
	if err := sc.Scan(&h.ID, &h.Name, &enabled, &events, &raw, &h.ScriptID, &quest, &h.RunOn, &h.TimeoutSec, &created, &updated,
		&h.ScopeKind, &scopeID, &h.CooldownSec, &h.MaxPerHour, &private, &h.DisabledReason); err != nil {
		return Hook{}, err
	}
	h.Enabled = enabled == 1
	h.IncludePrivate = private == 1
	if scopeID.Valid {
		v := scopeID.Int64
		h.ScopeID = &v
	}
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

// HookAreas are the kinds of area a hook may have.
var HookAreas = map[string]bool{"workspace": true, "quest": true, "step": true, "note": true, "note_tree": true}

// hookArea checks an area and the entity it names. A quest area keeps quest_id in step (it carries the cascade on delete).
func (s *Store) hookArea(ctx context.Context, kind string, id *int64) (string, *int64, *int64, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "workspace"
	}
	if !HookAreas[kind] {
		return "", nil, nil, errBad("scope_kind: workspace, quest, step, note или note_tree")
	}
	if kind == "workspace" {
		return kind, nil, nil, nil
	}
	if id == nil || *id < 1 {
		return "", nil, nil, errBad("scope_id: нужен номер сущности")
	}
	table := map[string]string{"quest": "quest", "step": "queststep", "note": "note", "note_tree": "note"}[kind]
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM `+table+` WHERE id = ?`, *id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil, errBad("сущность области не найдена")
	}
	if err != nil {
		return "", nil, nil, err
	}
	var quest *int64
	if kind == "quest" {
		v := *id
		quest = &v
	}
	v := *id
	return kind, &v, quest, nil
}

func checkHookLimits(cooldown, perHour int) error {
	if cooldown < 0 || cooldown > 86400 {
		return errBad("cooldown_sec: от 0 до 86400")
	}
	if perHour < 1 || perHour > 10000 {
		return errBad("max_per_hour: от 1 до 10000")
	}
	return nil
}

// CreateHook stores a new hook. questID is the old way to give it a quest area.
func (s *Store) CreateHook(ctx context.Context, name string, events []string, scriptID int64, questID *int64, runOn string, timeout int, enabled bool) (Hook, error) {
	return s.CreateHookIn(ctx, name, events, scriptID, questID, runOn, timeout, enabled, HookIn{})
}

// CreateHookIn is CreateHook with the area and the limits.
func (s *Store) CreateHookIn(ctx context.Context, name string, events []string, scriptID int64, questID *int64, runOn string, timeout int, enabled bool, in HookIn) (Hook, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > MaxHookName {
		return Hook{}, errBad("name: слишком длинное")
	}
	expanded, raw, runOnOut, timeoutOut, err := s.prepareHook(ctx, events, scriptID, runOn, timeout)
	if err != nil {
		return Hook{}, err
	}
	kind, scopeID := "", in.ScopeID
	if in.ScopeKind != nil {
		kind = *in.ScopeKind
	}
	if kind == "" && questID != nil {
		kind, scopeID = "quest", questID
	}
	kind, scopeID, quest, err := s.hookArea(ctx, kind, scopeID)
	if err != nil {
		return Hook{}, err
	}
	cooldown, perHour, private := 5, 120, false
	if in.CooldownSec != nil {
		cooldown = *in.CooldownSec
	}
	if in.MaxPerHour != nil {
		perHour = *in.MaxPerHour
	}
	if in.IncludePrivate != nil {
		private = *in.IncludePrivate
	}
	if err := checkHookLimits(cooldown, perHour); err != nil {
		return Hook{}, err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	res, err := s.DB.ExecContext(ctx, `INSERT INTO hook (name, enabled, events, events_raw, script_id, quest_id, run_on, timeout_sec, created_at, updated_at,
			scope_kind, scope_id, cooldown_sec, max_per_hour, include_private)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, boolInt(enabled), encodeJSONStrings(expanded), encodeJSONStrings(raw), scriptID, nullable(quest), runOnOut, timeoutOut, now, now,
		kind, nullable(scopeID), cooldown, perHour, boolInt(private))
	if err != nil {
		return Hook{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Hook{}, err
	}
	return s.GetHook(ctx, id)
}

// nullable turns a nil pointer into SQL NULL.
func nullable(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
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
	kind, scopeID := cur.ScopeKind, cur.ScopeID
	cooldown, perHour, private := cur.CooldownSec, cur.MaxPerHour, cur.IncludePrivate
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
	switch {
	case in.ScopeKind != nil:
		kind, scopeID = *in.ScopeKind, in.ScopeID
	case in.ClearQuest:
		kind, scopeID = "workspace", nil
	case in.QuestID != nil && *in.QuestID == 0:
		kind, scopeID = "workspace", nil
	case in.QuestID != nil:
		kind, scopeID = "quest", in.QuestID
	}
	if in.CooldownSec != nil {
		cooldown = *in.CooldownSec
	}
	if in.MaxPerHour != nil {
		perHour = *in.MaxPerHour
	}
	if in.IncludePrivate != nil {
		private = *in.IncludePrivate
	}
	expanded, raw, runOnOut, timeoutOut, err := s.prepareHook(ctx, events, scriptID, runOn, timeout)
	if err != nil {
		return Hook{}, err
	}
	kind, scopeID, quest, err := s.hookArea(ctx, kind, scopeID)
	if err != nil {
		return Hook{}, err
	}
	if err := checkHookLimits(cooldown, perHour); err != nil {
		return Hook{}, err
	}
	reason := cur.DisabledReason
	if enabled {
		reason = "" // switched on by hand: the old reason is gone
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	res, err := s.DB.ExecContext(ctx, `UPDATE hook SET name = ?, enabled = ?, events = ?, events_raw = ?, script_id = ?, quest_id = ?, run_on = ?, timeout_sec = ?, updated_at = ?,
			scope_kind = ?, scope_id = ?, cooldown_sec = ?, max_per_hour = ?, include_private = ?, disabled_reason = ?
		WHERE id = ?`, name, boolInt(enabled), encodeJSONStrings(expanded), encodeJSONStrings(raw), scriptID, nullable(quest), runOnOut, timeoutOut, now,
		kind, nullable(scopeID), cooldown, perHour, boolInt(private), reason, id)
	if err != nil {
		return Hook{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Hook{}, ErrNotFound
	}
	return s.GetHook(ctx, id)
}

// DisableHook switches a hook off from the dispatcher (a storm) and says why. The workspace is the one of ctx.
func (s *Store) DisableHook(ctx context.Context, id int64, reason string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE hook SET enabled = 0, disabled_reason = ?, updated_at = ? WHERE id = ?`,
		trimTo(reason, 300), timeutil.ToDBUTC(timeutil.NowUTC()), id)
	return err
}

// DeleteHook moves a hook to the trash (trash.go): it stops firing until it is restored.
func (s *Store) DeleteHook(ctx context.Context, id int64) error {
	return s.trashRow(ctx, "hook", id)
}

// HookEventsCatalog is the aliases and kinds the UI/API expose (same as CLI).
func HookEventsCatalog() map[string]any {
	aliases := hooks.EventAliases()
	seen := map[string]struct{}{}
	kinds := []string{}
	for _, e := range hooks.Catalog {
		seen[e.Kind] = struct{}{}
		kinds = append(kinds, e.Kind)
	}
	for _, targets := range aliases {
		for _, k := range targets {
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				kinds = append(kinds, k)
			}
		}
	}
	return map[string]any{"aliases": aliases, "kinds": kinds, "events": hooks.Catalog}
}

// EnabledHooks returns the enabled hooks of the workspace that wait for the kind of event (the dispatcher asks on every event,
// so no script data is fetched).
func (s *Store) EnabledHooks(ctx context.Context, kind string) ([]Hook, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+hookCols+` FROM hook h WHERE h.enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Hook
	for rows.Next() {
		h, err := scanHook(rows)
		if err != nil {
			return nil, err
		}
		for _, e := range h.Events {
			if e == kind {
				out = append(out, h)
				break
			}
		}
	}
	return out, rows.Err()
}

// NoteAncestors lists the parents of a note, nearest first (at most 32, a cycle cannot loop it).
func (s *Store) NoteAncestors(ctx context.Context, id int64) []int64 {
	var out []int64
	seen := map[int64]bool{id: true}
	for range maxValueChain {
		var parent sql.NullInt64
		if err := s.DB.QueryRowContext(ctx, `SELECT parent_id FROM note WHERE id = ?`, id).Scan(&parent); err != nil || !parent.Valid || seen[parent.Int64] {
			break
		}
		id = parent.Int64
		seen[id] = true
		out = append(out, id)
	}
	return out
}
