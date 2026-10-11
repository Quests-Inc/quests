package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/store"
)

// What an emit_pool_command prints: one quest, as a JSON object shaped like the
// body of POST /api/quests. The template supplies the defaults, the object
// overrides them. null / {} / nothing printed means "no quest this period".
//
// For templates written before this format, a JSON array of
// {title, description?, quest_description?} items is still accepted and turned
// into one quest whose steps are the items (weights, picks and refs are gone).

type emitStepSpec struct {
	Title                string
	Description          string
	ProgressTotal        *int
	ProgressCurrent      *int
	CheckCommand         *string
	CheckIntervalSeconds *int
	RunMode              string
	WaitPrevious         *bool
	RunOn                *string
}

type emitSpec struct {
	Title           *string
	Description     *string
	Significance    *string
	Pinned          *bool
	DeadlineAt      *string
	DurationSeconds *int
	Questline       json.RawMessage // id or name
	Category        json.RawMessage // id, slug or label
	Tags            json.RawMessage // [slug|id, …]
	Automated       *bool
	Steps           []emitStepSpec
	Ignored         []string // keys that are not part of the format (logged, not an error)
}

// emitOutput is the parsed stdout of one run.
type emitOutput struct {
	Spec    *emitSpec // set for a quest object
	Nothing bool
}

var specKeys = map[string]bool{
	"title": true, "description": true, "significance": true, "pinned": true,
	"deadline_at": true, "duration_seconds": true, "questline": true, "questline_id": true,
	"category": true, "category_id": true, "tags": true, "tag_ids": true,
	"automated": true, "steps": true,
}

var stepSpecKeys = map[string]bool{
	"title": true, "description": true, "progress_total": true, "progress_current": true,
	"check_command": true, "check_interval_seconds": true, "run_mode": true, "wait_previous": true,
	"run_on": true,
}

func parseEmitOutput(b []byte) (emitOutput, error) {
	s := bytes.TrimSpace(b)
	if len(s) == 0 || string(s) == "null" {
		return emitOutput{Nothing: true}, nil
	}
	switch s[0] {
	case '{':
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(s, &raw); err != nil {
			return emitOutput{}, fmt.Errorf("invalid JSON on stdout: %w", err)
		}
		if len(raw) == 0 {
			return emitOutput{Nothing: true}, nil
		}
		spec, err := decodeEmitSpec(raw)
		if err != nil {
			return emitOutput{}, err
		}
		return emitOutput{Spec: spec}, nil
	}
	return emitOutput{}, fmt.Errorf("invalid emit output: expected a JSON object (a quest) or null, got %q", truncate(string(s), 80))
}

func decodeEmitSpec(raw map[string]json.RawMessage) (*emitSpec, error) {
	spec := &emitSpec{}
	field := func(key string, into any) error {
		v, ok := raw[key]
		if !ok {
			return nil
		}
		if err := json.Unmarshal(v, into); err != nil {
			return fmt.Errorf("field %q: %v", key, err)
		}
		return nil
	}
	for key := range raw {
		if !specKeys[key] {
			spec.Ignored = append(spec.Ignored, key)
		}
	}
	for _, f := range []struct {
		key  string
		into any
	}{
		{"title", &spec.Title}, {"description", &spec.Description}, {"significance", &spec.Significance},
		{"pinned", &spec.Pinned}, {"deadline_at", &spec.DeadlineAt},
		{"duration_seconds", &spec.DurationSeconds}, {"automated", &spec.Automated},
	} {
		if err := field(f.key, f.into); err != nil {
			return nil, err
		}
	}
	for _, f := range []struct {
		keys []string
		into *json.RawMessage
	}{
		{[]string{"questline", "questline_id"}, &spec.Questline},
		{[]string{"category", "category_id"}, &spec.Category},
		{[]string{"tags", "tag_ids"}, &spec.Tags},
	} {
		for _, k := range f.keys {
			if v, ok := raw[k]; ok && string(bytes.TrimSpace(v)) != "null" {
				*f.into = v
				break
			}
		}
	}
	if v, ok := raw["steps"]; ok && string(bytes.TrimSpace(v)) != "null" {
		var rawSteps []map[string]json.RawMessage
		if err := json.Unmarshal(v, &rawSteps); err != nil {
			return nil, fmt.Errorf("field \"steps\": %v", err)
		}
		for i, rs := range rawSteps {
			var st emitStepSpec
			get := func(key string, into any) error {
				v, ok := rs[key]
				if !ok {
					return nil
				}
				if err := json.Unmarshal(v, into); err != nil {
					return fmt.Errorf("steps[%d].%s: %v", i, key, err)
				}
				return nil
			}
			for key := range rs {
				if !stepSpecKeys[key] {
					spec.Ignored = append(spec.Ignored, fmt.Sprintf("steps[%d].%s", i, key))
				}
			}
			for _, f := range []struct {
				key  string
				into any
			}{
				{"title", &st.Title}, {"description", &st.Description},
				{"progress_total", &st.ProgressTotal}, {"progress_current", &st.ProgressCurrent},
				{"check_command", &st.CheckCommand}, {"check_interval_seconds", &st.CheckIntervalSeconds},
				{"run_mode", &st.RunMode}, {"wait_previous", &st.WaitPrevious}, {"run_on", &st.RunOn},
			} {
				if err := get(f.key, f.into); err != nil {
					return nil, err
				}
			}
			spec.Steps = append(spec.Steps, st)
		}
	}
	sort.Strings(spec.Ignored)
	return spec, nil
}

// resolvedSpec is a validated emitSpec: limits enforced, names turned into ids.
// Zero values mean "use the template's".
type resolvedSpec struct {
	Title           string
	Description     string
	Significance    string
	Pinned          *bool
	DeadlineAt      *time.Time
	DurationSeconds *int
	QuestlineID     *int64
	CategoryID      *int64
	TagIDs          []int64
	TagsSet         bool
	Automated       *bool
	Steps           []journal.Step
	Ignored         []string
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// resolveEmitSpec validates spec against the template's limits and resolves
// questline/category/tags given by name. Any problem is an error: the run is
// treated like a failed attempt (logged, retried, then a failed quest).
func resolveEmitSpec(ctx context.Context, st *store.Store, spec *emitSpec, lim automation.EmitLimits) (*resolvedSpec, error) {
	rs := &resolvedSpec{Pinned: spec.Pinned, Automated: spec.Automated, Ignored: spec.Ignored}
	if spec.Title != nil {
		t := strings.TrimSpace(*spec.Title)
		if runeLen(t) > lim.MaxTitle {
			return nil, fmt.Errorf("title is %d characters, limit %d", runeLen(t), lim.MaxTitle)
		}
		rs.Title = t
	}
	if spec.Description != nil {
		if runeLen(*spec.Description) > lim.MaxDescription {
			return nil, fmt.Errorf("description is %d characters, limit %d", runeLen(*spec.Description), lim.MaxDescription)
		}
		rs.Description = *spec.Description
	}
	if spec.Significance != nil && strings.TrimSpace(*spec.Significance) != "" {
		sig := journal.Significance(strings.ToLower(strings.TrimSpace(*spec.Significance)))
		if !sig.Valid() {
			return nil, fmt.Errorf("%s", journal.InvalidSignificanceMsg(string(sig)))
		}
		rs.Significance = string(sig)
	}
	if spec.DeadlineAt != nil && strings.TrimSpace(*spec.DeadlineAt) != "" {
		t, err := timeutil.ParseFlexible(*spec.DeadlineAt)
		if err != nil {
			return nil, fmt.Errorf("deadline_at: %v", err)
		}
		rs.DeadlineAt = &t
	}
	if spec.DurationSeconds != nil {
		if *spec.DurationSeconds < 1 {
			return nil, fmt.Errorf("duration_seconds must be a positive integer")
		}
		d := *spec.DurationSeconds
		rs.DurationSeconds = &d
	}
	if len(spec.Steps) > lim.MaxSteps {
		return nil, fmt.Errorf("%d steps, limit %d", len(spec.Steps), lim.MaxSteps)
	}
	for i, ss := range spec.Steps {
		title := strings.TrimSpace(ss.Title)
		if title == "" {
			return nil, fmt.Errorf("steps[%d]: title required", i)
		}
		if runeLen(title) > lim.MaxTitle {
			return nil, fmt.Errorf("steps[%d]: title is %d characters, limit %d", i, runeLen(title), lim.MaxTitle)
		}
		if runeLen(ss.Description) > lim.MaxDescription {
			return nil, fmt.Errorf("steps[%d]: description is %d characters, limit %d", i, runeLen(ss.Description), lim.MaxDescription)
		}
		step := journal.Step{Title: title, Description: ss.Description, ProgressTotal: 1, SortOrder: i}
		if ss.ProgressTotal != nil {
			step.ProgressTotal = *ss.ProgressTotal
		}
		if ss.ProgressCurrent != nil {
			step.ProgressCurrent = *ss.ProgressCurrent
		}
		if ss.CheckCommand != nil && strings.TrimSpace(*ss.CheckCommand) != "" {
			if runeLen(*ss.CheckCommand) > lim.MaxCommand {
				return nil, fmt.Errorf("steps[%d]: check_command is %d characters, limit %d", i, runeLen(*ss.CheckCommand), lim.MaxCommand)
			}
			c := *ss.CheckCommand
			step.CheckCommand = &c
			step.CheckIntervalSeconds = ss.CheckIntervalSeconds
			if step.CheckIntervalSeconds != nil && *step.CheckIntervalSeconds < 15 {
				v := 15
				step.CheckIntervalSeconds = &v
			}
			step.RunMode = journal.NormalizeRunMode(ss.RunMode)
			if ss.WaitPrevious != nil {
				step.WaitPrevious = *ss.WaitPrevious
			}
			step.RunOn = store.NormalizeRunOn(ss.RunOn)
			if !store.ValidRunOn(step.RunOn) {
				return nil, fmt.Errorf("steps[%d]: run_on: %v", i, store.ErrBadRunnerLabel)
			}
		}
		journal.NormalizeStepCheck(&step)
		journal.ClampStep(&step)
		rs.Steps = append(rs.Steps, step)
	}

	var err error
	if rs.QuestlineID, err = resolveRefField(ctx, spec.Questline, "questline", st.Automation.ResolveQuestlineRef); err != nil {
		return nil, err
	}
	if rs.CategoryID, err = resolveRefField(ctx, spec.Category, "category", st.Automation.ResolveCategoryRef); err != nil {
		return nil, err
	}
	if len(spec.Tags) > 0 {
		var refs []any
		if err := json.Unmarshal(spec.Tags, &refs); err != nil {
			return nil, fmt.Errorf("tags: expected a list of slugs or ids: %v", err)
		}
		strs := make([]string, 0, len(refs))
		for _, r := range refs {
			switch v := r.(type) {
			case string:
				strs = append(strs, v)
			case float64:
				strs = append(strs, fmt.Sprintf("%d", int64(v)))
			default:
				return nil, fmt.Errorf("tags: expected slugs or ids, got %v", r)
			}
		}
		ids, err := st.Journal.ResolveTagIDs(ctx, strs)
		if err != nil {
			return nil, fmt.Errorf("tags: %v", err)
		}
		rs.TagIDs, rs.TagsSet = ids, true
	}
	return rs, nil
}

// resolveRefField reads an id or a name from a raw JSON value.
func resolveRefField(ctx context.Context, raw json.RawMessage, what string, resolve func(context.Context, string) (int64, error)) (*int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var text string
	var asNum json.Number
	if err := json.Unmarshal(raw, &text); err != nil {
		if err2 := json.Unmarshal(raw, &asNum); err2 != nil {
			return nil, fmt.Errorf("%s: expected an id or a name", what)
		}
		text = asNum.String()
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	id, err := resolve(ctx, text)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// applyResolvedSpec overlays what the command printed on the quest built from
// the template: whatever the quest object set wins, the rest stays the template's.
func applyResolvedSpec(ctx context.Context, st *store.Store, q *journal.Quest, tmpl templateRow, rs *resolvedSpec, now time.Time) {
	if rs.Title != "" {
		q.Title = rs.Title
	}
	if strings.TrimSpace(rs.Description) != "" {
		q.Description = rs.Description
	}
	if rs.Significance != "" {
		q.Significance = journal.Significance(rs.Significance)
	}
	if rs.Pinned != nil {
		q.Pinned = *rs.Pinned
		if q.Pinned {
			q.OverlayHidden = false
		}
	}
	if rs.Automated != nil {
		q.Automated = *rs.Automated
	}
	if rs.DeadlineAt != nil {
		q.DeadlineAt = rs.DeadlineAt
		switch {
		case rs.DurationSeconds != nil:
			q.DurationSeconds = rs.DurationSeconds
		case tmpl.DurationSeconds.Valid:
			d := int(tmpl.DurationSeconds.Int64)
			q.DurationSeconds = &d
		default:
			d := timeutil.AutoDurationSeconds(*rs.DeadlineAt, now)
			q.DurationSeconds = &d
		}
	} else if rs.DurationSeconds != nil && q.DeadlineAt != nil {
		q.DurationSeconds = rs.DurationSeconds
	}
	if rs.QuestlineID != nil {
		q.QuestlineID = rs.QuestlineID
		if rs.CategoryID == nil {
			// a questline brings its own category, as everywhere else
			if line, err := st.Journal.GetQuestline(ctx, *rs.QuestlineID); err == nil {
				if line.CategoryID != nil {
					c := *line.CategoryID
					q.CategoryID = &c
				}
			}
		}
	}
	if rs.CategoryID != nil {
		q.CategoryID = rs.CategoryID
	}
}

// attachTags gives the new quest the tags the command printed, or the
// template's own when it printed none.
func attachTags(ctx context.Context, st *store.Store, tmpl templateRow, questID int64, rs *resolvedSpec) error {
	if rs != nil && rs.TagsSet {
		return st.Journal.SetQuestTags(ctx, questID, rs.TagIDs)
	}
	return st.Journal.CopyTemplateTagsToQuest(ctx, tmpl.ID, questID)
}

// followQuestlineCategory enforces the same rule as the REST API: a quest in a
// questline always carries that questline's category, whatever the template or
// the emitted spec said. A questline without a category leaves it alone.
func followQuestlineCategory(ctx context.Context, st *store.Store, q *journal.Quest) {
	if q.QuestlineID == nil {
		return
	}
	if c, err := st.Journal.QuestlineCategory(ctx, *q.QuestlineID); err == nil && c != nil {
		q.CategoryID = c
	}
}
