package schedule

import (
	"context"

	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// publishTemplateEvent tells the hooks what happened to a template's period: it gave a quest (`template_emitted`), its emit
// ran out of attempts (`template_emit_failed`) or the period passed with nothing to give (`template_period_skipped`). Quiet:
// these are for hooks, not for the screen.
func publishTemplateEvent(ctx context.Context, hub *events.Hub, kind string, tmpl templateRow, period, detail string, questID *int64) {
	if hub == nil {
		return
	}
	silent := ""
	extra := map[string]any{"template_id": tmpl.ID, "period": period}
	hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(ctx), QuestID: questID, Title: tmpl.Title, Detail: detail,
		Toast: false, Source: "system", Sound: &silent, Extra: extra,
	})
}
