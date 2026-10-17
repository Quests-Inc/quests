package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

const hookJobValidFor = 24 * time.Hour

// DispatchHooks starts a script run for every enabled workspace hook that matches the event.
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
	qid := eventQuestID(p)
	list, err := r.Store.Execution.HooksForEvent(ctx, kind, qid)
	if err != nil {
		log.Printf("hooks: list: %v", err)
		return
	}
	for _, h := range list {
		if err := r.startHook(ctx, h, p, ws); err != nil {
			log.Printf("hooks: hook %d: %v", h.ID, err)
		}
	}
}

func (r *CheckRunner) startHook(ctx context.Context, h execution.Hook, event events.Payload, workspaceID int64) error {
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
	_, err = r.startRun(ctx, ref, &sid, g, runSubject{Type: subjType, ID: subjID, Event: ev}, runBinding{
		Label: h.RunOn, Kind: execution.JobKindHook, Title: title,
		Dedupe:  "hook:" + strconv.FormatInt(h.ID, 10) + ":" + rev,
		Timeout: timeout, ValidFor: hookJobValidFor, QuestID: questID, Env: env,
	})
	return err
}

// hookSubject picks the entity the graph's source reads. A quest-scoped hook always runs as that quest;
// a workspace hook follows the graph (quest when the event names one, else workspace).
func hookSubject(g graph.Graph, h execution.Hook, event events.Payload, workspaceID int64) (string, int64, error) {
	want := graphSubject(g)
	if h.QuestID != nil {
		if want != "" && want != "quest" {
			return "", 0, fmt.Errorf("скрипт читает %s, а хук привязан к квесту", want)
		}
		return "quest", *h.QuestID, nil
	}
	switch want {
	case "quest":
		qid := eventQuestID(event)
		if qid == nil {
			return "", 0, fmt.Errorf("скрипт читает квест, а у события нет quest_id")
		}
		return "quest", *qid, nil
	case "workspace", "":
		return "workspace", workspaceID, nil
	default:
		return "", 0, fmt.Errorf("хук не умеет источник %s", want)
	}
}

func graphSubject(g graph.Graph) string {
	for _, n := range g.Nodes {
		if kind, ok := strings.CutPrefix(n.Type, "source."); ok {
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
	}
	return nil
}
