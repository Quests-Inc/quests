package journalhttp

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func (h *Handler) getQuestLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	i64 := func(key string) *int64 {
		if v := q.Get(key); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return &n
			}
		}
		return nil
	}
	list := func(key string) []string {
		var out []string
		for _, p := range strings.Split(q.Get(key), ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	when := func(key string) *time.Time {
		if v := q.Get(key); v != "" {
			if t, err := timeutil.ParseFlexible(v); err == nil {
				return &t
			}
		}
		return nil
	}
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := h.Store.ListChangeLog(r.Context(), journal.ChangeLogFilter{
		Limit:       limit,
		QuestID:     i64("quest_id"),
		EntityTypes: list("entity_type"),
		EntityID:    i64("entity_id"),
		OwnerType:   q.Get("owner_type"),
		OwnerID:     i64("owner_id"),
		Kinds:       list("kind"),
		QuestlineID: i64("questline_id"),
		CategoryID:  i64("category_id"),
		TagID:       i64("tag_id"),
		From:        when("from"),
		To:          when("to"),
		Query:       q.Get("q"),
		BeforeID:    i64("before_id"),
		BeforeAt:    when("before_at"),
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}
