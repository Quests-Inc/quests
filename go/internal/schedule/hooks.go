package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

const hookJobValidFor = 24 * time.Hour

// originKeyPrefix marks the run key that carries the chain of hooks a run descends from.
const originKeyPrefix = "origin:"

// maxHookDepth is how long a chain of hooks waking hooks may be before it is cut.
const maxHookDepth = 3

// eventRefs are the entities an event is about, whatever its kind.
type eventRefs struct {
	Quest, Step, Note, Template int64
	OwnerKind                   string
	OwnerID                     int64
}

func eventInt(event events.Payload, key string) int64 {
	switch v := event[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case *int64:
		// The hub hands its observers the payload as it was built: the quest is a pointer there, not yet a JSON number.
		if v != nil {
			return *v
		}
	}
	return 0
}

func refsOf(event events.Payload) eventRefs {
	r := eventRefs{Quest: eventInt(event, "quest_id"), Step: eventInt(event, "step_id"), Note: eventInt(event, "note_id"), Template: eventInt(event, "template_id")}
	r.OwnerKind, _ = event["owner_kind"].(string)
	r.OwnerID = eventInt(event, "owner_id")
	if r.Note == 0 && r.OwnerKind == "note" {
		r.Note = r.OwnerID // a variable of a note: the note is what it is about
	}
	if r.Template == 0 && r.OwnerKind == "template" {
		r.Template = r.OwnerID
	}
	if r.Step == 0 && r.OwnerKind == "step" {
		r.Step = r.OwnerID
	}
	if r.Quest == 0 && r.OwnerKind == "quest" {
		r.Quest = r.OwnerID
	}
	return r
}

// entityKey names what the event is about, for the pause between two runs.
func (e eventRefs) entityKey() string {
	switch {
	case e.Note > 0:
		return "note=" + strconv.FormatInt(e.Note, 10)
	case e.Step > 0:
		return "step=" + strconv.FormatInt(e.Step, 10)
	case e.Quest > 0:
		return "quest=" + strconv.FormatInt(e.Quest, 10)
	case e.Template > 0:
		return "template=" + strconv.FormatInt(e.Template, 10)
	case e.OwnerID > 0:
		return e.OwnerKind + "=" + strconv.FormatInt(e.OwnerID, 10)
	}
	return "-"
}

// originChain is the hooks an event descends from, oldest first.
func originChain(event events.Payload) []string {
	s, _ := event["origin"].(string)
	if s == "" {
		return nil
	}
	return strings.Split(s, ">")
}

// DispatchHooks starts a script run for every enabled workspace hook that matches the event: its kind, its area, and the limits
// of the hook (a pause per entity, a number per hour, no chain of hooks waking hooks, no private notes unless asked for).
func (r *CheckRunner) DispatchHooks(p events.Payload) {
	kind, _ := p["kind"].(string)
	if kind == "" || kind == "startup" {
		return
	}
	ws := eventWorkspaceID(p)
	if ws <= 0 {
		ws = 1
	}
	ctx := wsctx.With(context.Background(), ws)
	list, err := r.Store.Execution.EnabledHooks(ctx, kind)
	if err != nil {
		log.Printf("hooks: list: %v", err)
		return
	}
	if len(list) == 0 {
		return
	}
	refs := refsOf(p)
	chain := originChain(p)
	var private *bool
	for _, h := range list {
		if !r.hookInArea(ctx, h, refs) {
			continue
		}
		if len(chain) >= maxHookDepth || slices.Contains(chain, "hook:"+strconv.FormatInt(h.ID, 10)) {
			continue // the event was made by this hook's own work, or by a chain too long to be anything but a loop
		}
		if refs.Note > 0 && !h.IncludePrivate {
			if private == nil {
				v, err := r.Store.Execution.NotePrivate(ctx, refs.Note)
				private = &v
				if err != nil {
					*private = false
				}
			}
			if *private {
				continue
			}
		}
		h, p := h, p
		r.hookGate.submit(h, kind, refs.entityKey(), func(over string) {
			if over != "" {
				if err := r.Store.Execution.DisableHook(ctx, h.ID, over); err != nil {
					log.Printf("hooks: switch off %d: %v", h.ID, err)
				}
				log.Printf("hooks: hook %d switched off: %s", h.ID, over)
				return
			}
			if err := r.startHook(ctx, h, p, ws, chain); err != nil {
				log.Printf("hooks: hook %d: %v", h.ID, err)
			}
		})
	}
}

// hookInArea says whether the event is inside the area of the hook.
func (r *CheckRunner) hookInArea(ctx context.Context, h execution.Hook, e eventRefs) bool {
	switch h.ScopeKind {
	case "", "workspace":
		return true
	}
	if h.ScopeID == nil {
		return false
	}
	id := *h.ScopeID
	switch h.ScopeKind {
	case "quest":
		return e.Quest == id
	case "step":
		return e.Step == id
	case "note":
		return e.Note == id
	case "note_tree":
		if e.Note == 0 {
			return false
		}
		if e.Note == id {
			return true
		}
		return slices.Contains(r.Store.Execution.NoteAncestors(ctx, e.Note), id)
	}
	return false
}

func (r *CheckRunner) startHook(ctx context.Context, h execution.Hook, event events.Payload, workspaceID int64, chain []string) error {
	sc, err := r.Store.Execution.GetScript(ctx, h.ScriptID)
	if err != nil {
		return err
	}
	g, err := graph.Decode(sc.Graph)
	if err != nil {
		return err
	}
	ref := fmt.Sprintf("script:%d:%d", sc.ID, sc.Version)
	sid := sc.ID

	subjType, subjID, err := hookSubject(g, h, event, workspaceID)
	if err != nil {
		return err
	}

	rev := fmt.Sprint(event["revision"])
	timeout := time.Duration(h.TimeoutSec) * time.Second
	if timeout < time.Second {
		timeout = 30 * time.Second
	}
	title := h.Name
	if title == "" {
		title = "хук · " + sc.Title
	}
	var questID *int64
	if subjType == "quest" {
		questID = &subjID
	} else if q := eventQuestID(event); q != nil {
		questID = q
	}
	ev := map[string]any(event)
	payload, _ := json.Marshal(event)
	env := map[string]string{
		"QUESTS_KIND":    fmt.Sprint(event["kind"]),
		"QUESTS_TITLE":   fmt.Sprint(event["title"]),
		"QUESTS_DETAIL":  fmt.Sprint(event["detail"]),
		"QUESTS_PAYLOAD": string(payload),
	}
	if questID != nil {
		env["QUESTS_QUEST_ID"] = strconv.FormatInt(*questID, 10)
	}
	origin := strings.Join(append(slices.Clone(chain), "hook:"+strconv.FormatInt(h.ID, 10)), ">")
	_, err = r.startRun(ctx, ref, &sid, g, runSubject{Type: subjType, ID: subjID, Event: ev, Origin: origin, SourceKind: execution.RunSourceHook, SourceID: h.ID}, runBinding{
		Label: h.RunOn, Kind: execution.JobKindHook, Title: title,
		Dedupe:  "hook:" + strconv.FormatInt(h.ID, 10) + ":" + rev,
		Timeout: timeout, ValidFor: hookJobValidFor, QuestID: questID, Env: env,
	})
	return err
}

// hookSubject picks the entity the graph's source reads. A hook with an area of one quest, step or note runs as that entity; a
// workspace hook follows the graph (the entity the event names, else the workspace). A graph that reads only the event
// (`source.event`) needs none and runs as the workspace.
func hookSubject(g graph.Graph, h execution.Hook, event events.Payload, workspaceID int64) (string, int64, error) {
	want := graphSubject(g)
	refs := refsOf(event)
	pick := func(kind string) (string, int64, error) {
		var id int64
		switch kind {
		case "quest":
			id = refs.Quest
		case "step":
			id = refs.Step
		case "note":
			id = refs.Note
		case "template":
			id = refs.Template
		default:
			return "", 0, fmt.Errorf("хук не умеет источник %s", kind)
		}
		if id == 0 {
			return "", 0, fmt.Errorf("скрипт читает %s, а у события нет такой сущности", kind)
		}
		return kind, id, nil
	}
	if h.ScopeID != nil && (h.ScopeKind == "quest" || h.ScopeKind == "step" || h.ScopeKind == "note") {
		if want != "" && want != h.ScopeKind {
			return "", 0, fmt.Errorf("скрипт читает %s, а хук привязан к области %s", want, h.ScopeKind)
		}
		if want == "" {
			return "workspace", workspaceID, nil
		}
		return h.ScopeKind, *h.ScopeID, nil
	}
	switch want {
	case "workspace", "":
		return "workspace", workspaceID, nil
	}
	return pick(want)
}

func graphSubject(g graph.Graph) string {
	for _, n := range g.Nodes {
		if kind, ok := strings.CutPrefix(n.Type, "source."); ok && kind != "event" {
			return kind
		}
	}
	return ""
}

func eventWorkspaceID(event events.Payload) int64 {
	switch v := event["workspace"].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

func eventQuestID(event events.Payload) *int64 {
	switch v := event["quest_id"].(type) {
	case float64:
		i := int64(v)
		if i > 0 {
			return &i
		}
	case int64:
		if v > 0 {
			return &v
		}
	case int:
		i := int64(v)
		if i > 0 {
			return &i
		}
	case *int64:
		if v != nil && *v > 0 {
			i := *v
			return &i
		}
	}
	return nil
}
