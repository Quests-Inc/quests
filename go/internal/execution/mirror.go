package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// A step that follows another quest (quest=298) is made of the parts hooks already have: a hook whose area is the followed
// quest, waiting for what changes its steps, and a script that reads the quest's progress and writes it onto the step. The
// hook knows the step it feeds (`mirror_step_id`) and goes away with it; the script is made for this one link.

// Mirror is the link of one step to the quest it follows.
type Mirror struct {
	StepID        int64 `json:"step_id"`
	QuestID       int64 `json:"quest_id"`
	ParentQuestID int64 `json:"parent_quest_id"`
	HookID        int64 `json:"hook_id"`
	ScriptID      int64 `json:"script_id"`
}

// mirrorEvents are what changes the progress of a quest: a step moved, the status changed, steps came or went.
var mirrorEvents = []string{"step_progress", "step_completed", "quest_updated", "status_changed", "quest_completed", "quest_failed", "quest_expired"}

func mirrorGraph(stepID int64) []byte {
	g := map[string]any{
		"nodes": []any{
			map[string]any{"id": 1, "type": "source.quest"},
			map[string]any{"id": 2, "type": "source.step", "params": map[string]any{"id": stepID}},
			map[string]any{"id": 3, "type": "sink.step_progress"},
		},
		"edges": []any{
			map[string]any{"from": map[string]any{"node": 1, "port": "progress"}, "to": map[string]any{"node": 3, "port": "progress"}},
			map[string]any{"from": map[string]any{"node": 1, "port": "total"}, "to": map[string]any{"node": 3, "port": "total"}},
			map[string]any{"from": map[string]any{"node": 2, "port": "ref"}, "to": map[string]any{"node": 3, "port": "ref"}},
		},
	}
	b, _ := json.Marshal(g)
	return b
}

// StepMirror returns the link of a step, or nil when it follows nothing.
func (s *Store) StepMirror(ctx context.Context, stepID int64) (*Mirror, error) {
	var m Mirror
	err := s.DB.QueryRowContext(ctx, `SELECT h.id, h.script_id, h.scope_id, qs.quest_id FROM hook h
		JOIN queststep qs ON qs.id = h.mirror_step_id WHERE h.mirror_step_id = ?`, stepID).Scan(&m.HookID, &m.ScriptID, &m.QuestID, &m.ParentQuestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.StepID = stepID
	return &m, nil
}

// mirrorsFollowing maps a quest to the quests whose steps it follows.
func (s *Store) mirrorsFollowing(ctx context.Context) (map[int64][]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT qs.quest_id, h.scope_id FROM hook h
		JOIN queststep qs ON qs.id = h.mirror_step_id
		WHERE h.mirror_step_id IS NOT NULL AND h.scope_kind = 'quest' AND h.scope_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]int64{}
	for rows.Next() {
		var parent, followed int64
		if err := rows.Scan(&parent, &followed); err != nil {
			return nil, err
		}
		out[parent] = append(out[parent], followed)
	}
	return out, rows.Err()
}

// reaches says whether `from` follows `target`, directly or through the quests it follows.
func reaches(links map[int64][]int64, from, target int64) bool {
	seen := map[int64]bool{}
	var walk func(int64) bool
	walk = func(q int64) bool {
		if q == target {
			return true
		}
		if seen[q] {
			return false
		}
		seen[q] = true
		for _, next := range links[q] {
			if walk(next) {
				return true
			}
		}
		return false
	}
	return walk(from)
}

// SetStepMirror makes a step follow a quest, replacing what it followed before. It refuses a step that would follow its own
// quest or close a circle (A follows B, B follows A, however many quests lie between).
func (s *Store) SetStepMirror(ctx context.Context, stepID, questID int64) (Mirror, error) {
	var parent int64
	err := s.DB.QueryRowContext(ctx, `SELECT quest_id FROM queststep WHERE id = ?`, stepID).Scan(&parent)
	if errors.Is(err, sql.ErrNoRows) {
		return Mirror{}, errBad("шаг не найден")
	}
	if err != nil {
		return Mirror{}, err
	}
	var one int
	err = s.DB.QueryRowContext(ctx, `SELECT 1 FROM quest WHERE id = ?`, questID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return Mirror{}, errBad("квест не найден")
	}
	if err != nil {
		return Mirror{}, err
	}
	if questID == parent {
		return Mirror{}, errBad("шаг не может следить за своим квестом")
	}
	links, err := s.mirrorsFollowing(ctx)
	if err != nil {
		return Mirror{}, err
	}
	if reaches(links, questID, parent) {
		return Mirror{}, errBad(fmt.Sprintf("получился бы круг: квест %d уже следит за квестом %d", questID, parent))
	}
	if err := s.ClearStepMirror(ctx, stepID); err != nil {
		return Mirror{}, err
	}
	title := fmt.Sprintf("Слежение: шаг %d за квестом %d", stepID, questID)
	sc, err := s.CreateScriptFor(ctx, title, "Создан полем «Следит за квестом» у шага. Правится и убирается там же.", "", mirrorGraph(stepID))
	if err != nil {
		return Mirror{}, err
	}
	scope, cooldown, perHour := "quest", 2, 2000
	h, err := s.CreateHookIn(ctx, title, mirrorEvents, sc.ID, nil, "", 30, true, HookIn{ScopeKind: &scope, ScopeID: &questID, CooldownSec: &cooldown, MaxPerHour: &perHour})
	if err != nil {
		_ = s.DeleteScript(ctx, sc.ID)
		return Mirror{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE hook SET mirror_step_id = ? WHERE id = ?`, stepID, h.ID); err != nil {
		return Mirror{}, err
	}
	return Mirror{StepID: stepID, QuestID: questID, ParentQuestID: parent, HookID: h.ID, ScriptID: sc.ID}, nil
}

// ClearStepMirror stops a step from following anything: the hook goes, and its script with it.
func (s *Store) ClearStepMirror(ctx context.Context, stepID int64) error {
	m, err := s.StepMirror(ctx, stepID)
	if err != nil || m == nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM hook WHERE id = ?`, m.HookID); err != nil {
		return err
	}
	if err := s.DeleteScript(ctx, m.ScriptID); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}
