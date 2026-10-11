package llmassist

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/valirum/quests/go/internal/platform/apiclient"
)

// ActionExecError carries the offending action's index alongside the message.
type ActionExecError struct {
	Index int
	Msg   string
}

func (e *ActionExecError) Error() string { return fmt.Sprintf("action[%d]: %s", e.Index, e.Msg) }

func execErrf(index int, format string, a ...any) error {
	return &ActionExecError{Index: index, Msg: fmt.Sprintf(format, a...)}
}

// ActionResult is one executed (or dry-run-previewed) action's outcome.
type ActionResult struct {
	Index  int            `json:"index"`
	Action string         `json:"action"`
	IsNew  bool           `json:"is_new"`
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
	Result map[string]any `json:"result,omitempty"`
}

func placeholderID(index int) int64 { return -(int64(index) + 1) }

// Executor applies an ActionBatch against the same HTTP API the frontend and
// CLI use — via loopback (127.0.0.1), not a separate process. Reuses
// apiclient.Client for requests and its ResolveCategoryID/ResolveQuestlineID for
// the identical name/substring resolution the Go CLI already has.
type Executor struct {
	Client *apiclient.Client
}

// NewExecutor acts in workspace ws (the caller's): the loopback token is not a
// user, so without an explicit workspace it would land in the default one.
func NewExecutor(apiBase, token string, ws int64) *Executor {
	c := apiclient.NewClient(apiBase, true)
	// Loopback calls to our own API must carry credentials once accounts are on.
	c.Token = token
	c.Source = "assistant"
	if ws > 0 {
		c.Workspace = strconv.FormatInt(ws, 10)
	}
	return &Executor{Client: c}
}

func (e *Executor) Run(batch ActionBatch, dryRun bool) ([]ActionResult, error) {
	if batch.NeedsClarification {
		return nil, fmt.Errorf("needs_clarification: %s", batch.ClarifyQuestion)
	}
	results := make([]ActionResult, 0, len(batch.Actions))
	for _, act := range batch.Actions {
		r, err := e.runOne(act, results, dryRun)
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}

func (e *Executor) resolvedID(literal *int64, ref *int, results []ActionResult, label string) (*int64, error) {
	if ref == nil {
		return literal, nil
	}
	idx := *ref
	if idx < 0 || idx >= len(results) {
		return nil, fmt.Errorf("%s_ref points to an unresolved action", label)
	}
	src := results[idx].Result
	if src == nil {
		src = results[idx].After
	}
	id, ok := apiclient.AsInt64(src["id"])
	if !ok {
		return nil, fmt.Errorf("%s_ref target has no id", label)
	}
	return &id, nil
}

func (e *Executor) resolveCategory(raw *string) (*int64, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	return e.Client.ResolveCategoryID(*raw)
}

func (e *Executor) resolveQuestlineByName(raw *string) (*int64, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	return e.Client.ResolveQuestlineID(*raw)
}

func toMap(raw json.RawMessage) (map[string]any, error) {
	return apiclient.DecodeMap(raw)
}

func (e *Executor) getQuest(id int64) (map[string]any, error) {
	raw, err := e.Client.Get(fmt.Sprintf("/api/quests/%d", id), nil)
	if err != nil {
		return nil, err
	}
	return toMap(raw)
}

func (e *Executor) runOne(act Action, results []ActionResult, dryRun bool) (ActionResult, error) {
	questID, err := e.resolvedID(act.QuestID, act.QuestIDRef, results, "quest")
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	questlineID, err := e.resolvedID(act.QuestlineID, act.QuestlineIDRef, results, "questline")
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}

	switch act.Action {
	case "create_questline":
		return e.createQuestline(act, dryRun)
	case "create_quest":
		return e.createQuest(act, questlineID, dryRun)
	case "add_step":
		return e.addStep(act, questID, dryRun)
	case "update_quest":
		return e.updateQuest(act, questID, questlineID, dryRun)
	case "update_step":
		return e.updateStep(act, questID, dryRun)
	case "delete_step":
		return e.deleteStep(act, questID, dryRun)
	default:
		return ActionResult{}, execErrf(act.Index, "unknown action %q", act.Action)
	}
}

func (e *Executor) createQuestline(act Action, dryRun bool) (ActionResult, error) {
	if act.Title == nil || *act.Title == "" {
		return ActionResult{}, execErrf(act.Index, "create_questline requires title")
	}
	body := map[string]any{"title": *act.Title}
	if act.Description != nil {
		body["description"] = *act.Description
	}
	if act.Color != nil {
		body["color"] = *act.Color
	}
	if act.Icon != nil {
		body["icon"] = *act.Icon
	}
	if act.CategoryID != nil {
		body["category_id"] = *act.CategoryID
	} else {
		catID, err := e.resolveCategory(act.Category)
		if err != nil {
			return ActionResult{}, execErrf(act.Index, "%s", err)
		}
		if catID != nil {
			body["category_id"] = *catID
		}
	}

	if dryRun {
		after := map[string]any{"id": placeholderID(act.Index), "title": *act.Title}
		return ActionResult{Index: act.Index, Action: act.Action, IsNew: true, After: after}, nil
	}
	raw, err := e.Client.Post("/api/questlines", nil, body)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	m, err := toMap(raw)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	return ActionResult{Index: act.Index, Action: act.Action, IsNew: true, After: m, Result: m}, nil
}

func (e *Executor) createQuest(act Action, questlineID *int64, dryRun bool) (ActionResult, error) {
	if act.Title == nil || *act.Title == "" {
		return ActionResult{}, execErrf(act.Index, "create_quest requires title")
	}
	body := map[string]any{"title": *act.Title}
	if act.Description != nil {
		body["description"] = *act.Description
	}
	if act.Significance != nil {
		body["significance"] = *act.Significance
	}
	if act.Pinned != nil {
		body["pinned"] = *act.Pinned
	}
	if act.SortOrder != nil {
		body["sort_order"] = *act.SortOrder
	}
	if act.DeadlineAt != nil {
		body["deadline_at"] = *act.DeadlineAt
	}
	if act.DurationSeconds != nil {
		body["duration_seconds"] = *act.DurationSeconds
	}
	if questlineID != nil {
		body["questline_id"] = *questlineID
	} else if act.CategoryID != nil {
		body["category_id"] = *act.CategoryID
	} else {
		catID, err := e.resolveCategory(act.Category)
		if err != nil {
			return ActionResult{}, execErrf(act.Index, "%s", err)
		}
		if catID != nil {
			body["category_id"] = *catID
		}
	}
	if len(act.Steps) > 0 {
		steps := make([]map[string]any, 0, len(act.Steps))
		for _, s := range act.Steps {
			total := s.ProgressTotal
			if total < 1 {
				total = 1
			}
			steps = append(steps, map[string]any{
				"title": s.Title, "description": s.Description,
				"progress_total": total, "progress_current": s.ProgressCurrent,
			})
		}
		body["steps"] = steps
	}

	if dryRun {
		after := map[string]any{"id": placeholderID(act.Index), "title": *act.Title}
		for k, v := range body {
			if k != "title" && k != "steps" {
				after[k] = v
			}
		}
		return ActionResult{Index: act.Index, Action: act.Action, IsNew: true, After: after}, nil
	}
	raw, err := e.Client.Post("/api/quests", nil, body)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	m, err := toMap(raw)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	return ActionResult{Index: act.Index, Action: act.Action, IsNew: true, After: m, Result: m}, nil
}

func (e *Executor) addStep(act Action, questID *int64, dryRun bool) (ActionResult, error) {
	if questID == nil {
		return ActionResult{}, execErrf(act.Index, "add_step requires quest_id or quest_id_ref")
	}
	if *questID < 0 {
		return ActionResult{}, execErrf(act.Index, "target quest only exists as a dry-run placeholder; add_step on it is shown nested under that quest, not fetched")
	}
	if act.Title == nil || *act.Title == "" {
		return ActionResult{}, execErrf(act.Index, "add_step requires title")
	}
	before, err := e.getQuest(*questID)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	total := 1
	if act.ProgressTotal != nil {
		total = *act.ProgressTotal
	}
	if total < 1 {
		total = 1
	}
	current := 0
	if act.ProgressCurrent != nil {
		current = *act.ProgressCurrent
	}
	body := map[string]any{"title": *act.Title, "progress_total": total, "progress_current": current}
	if act.Description != nil {
		body["description"] = *act.Description
	}
	if act.SortOrder != nil {
		body["sort_order"] = *act.SortOrder
	}

	after := map[string]any{"title": *act.Title, "quest_id": *questID}
	if dryRun {
		return ActionResult{Index: act.Index, Action: act.Action, IsNew: true, Before: before, After: after}, nil
	}
	raw, err := e.Client.Post(fmt.Sprintf("/api/quests/%d/steps", *questID), nil, body)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	m, err := toMap(raw)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	return ActionResult{Index: act.Index, Action: act.Action, IsNew: true, Before: before, After: after, Result: m}, nil
}

func (e *Executor) updateQuest(act Action, questID, questlineID *int64, dryRun bool) (ActionResult, error) {
	if questID == nil {
		return ActionResult{}, execErrf(act.Index, "update_quest requires quest_id or quest_id_ref")
	}
	if *questID < 0 {
		return ActionResult{}, execErrf(act.Index, "target quest only exists as a dry-run placeholder; fold this update into that create_quest action instead")
	}
	before, err := e.getQuest(*questID)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	body := map[string]any{}
	if act.Title != nil {
		body["title"] = *act.Title
	}
	if act.Description != nil {
		body["description"] = *act.Description
	}
	if act.Status != nil {
		body["status"] = *act.Status
	}
	if act.Significance != nil {
		body["significance"] = *act.Significance
	}
	if act.Pinned != nil {
		body["pinned"] = *act.Pinned
	}
	if act.SortOrder != nil {
		body["sort_order"] = *act.SortOrder
	}
	if act.DeadlineAt != nil {
		body["deadline_at"] = *act.DeadlineAt
	}
	if act.DurationSeconds != nil {
		body["duration_seconds"] = *act.DurationSeconds
	}
	if act.CategoryID != nil {
		body["category_id"] = *act.CategoryID
	}
	if act.ClearQuestline {
		body["questline_id"] = nil
	} else if questlineID != nil {
		body["questline_id"] = *questlineID
	} else if act.Questline != nil {
		id, err := e.resolveQuestlineByName(act.Questline)
		if err != nil {
			return ActionResult{}, execErrf(act.Index, "%s", err)
		}
		if id != nil {
			body["questline_id"] = *id
		}
	}
	if len(body) == 0 {
		return ActionResult{}, execErrf(act.Index, "no fields to change")
	}

	after := map[string]any{"id": *questID}
	for k, v := range body {
		after[k] = v
	}
	if dryRun {
		return ActionResult{Index: act.Index, Action: act.Action, IsNew: false, Before: before, After: after}, nil
	}
	raw, err := e.Client.Patch(fmt.Sprintf("/api/quests/%d", *questID), nil, body)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	m, err := toMap(raw)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	return ActionResult{Index: act.Index, Action: act.Action, IsNew: false, Before: before, After: after, Result: m}, nil
}

func (e *Executor) updateStep(act Action, questID *int64, dryRun bool) (ActionResult, error) {
	if questID == nil {
		return ActionResult{}, execErrf(act.Index, "update_step requires quest_id or quest_id_ref")
	}
	if act.StepID == nil {
		return ActionResult{}, execErrf(act.Index, "update_step requires step_id")
	}
	before, err := e.getQuest(*questID)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	body := map[string]any{}
	if act.Title != nil {
		body["title"] = *act.Title
	}
	if act.Description != nil {
		body["description"] = *act.Description
	}
	if act.ProgressCurrent != nil {
		body["progress_current"] = *act.ProgressCurrent
	}
	if act.ProgressTotal != nil {
		body["progress_total"] = *act.ProgressTotal
	}
	if act.SortOrder != nil {
		body["sort_order"] = *act.SortOrder
	}
	if len(body) == 0 {
		return ActionResult{}, execErrf(act.Index, "no fields to change")
	}
	after := map[string]any{"quest_id": *questID, "step_id": *act.StepID}
	for k, v := range body {
		after[k] = v
	}
	if dryRun {
		return ActionResult{Index: act.Index, Action: act.Action, IsNew: false, Before: before, After: after}, nil
	}
	raw, err := e.Client.Patch(fmt.Sprintf("/api/quests/%d/steps/%d", *questID, *act.StepID), nil, body)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	m, err := toMap(raw)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	return ActionResult{Index: act.Index, Action: act.Action, IsNew: false, Before: before, After: after, Result: m}, nil
}

func (e *Executor) deleteStep(act Action, questID *int64, dryRun bool) (ActionResult, error) {
	if questID == nil {
		return ActionResult{}, execErrf(act.Index, "delete_step requires quest_id or quest_id_ref")
	}
	if act.StepID == nil {
		return ActionResult{}, execErrf(act.Index, "delete_step requires step_id")
	}
	before, err := e.getQuest(*questID)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	after := map[string]any{"quest_id": *questID, "step_id": *act.StepID, "deleted": true}
	if dryRun {
		return ActionResult{Index: act.Index, Action: act.Action, IsNew: false, Before: before, After: after}, nil
	}
	raw, err := e.Client.Delete(fmt.Sprintf("/api/quests/%d/steps/%d", *questID, *act.StepID), nil)
	if err != nil {
		return ActionResult{}, execErrf(act.Index, "%s", err)
	}
	m, _ := toMap(raw)
	return ActionResult{Index: act.Index, Action: act.Action, IsNew: false, Before: before, After: after, Result: m}, nil
}
