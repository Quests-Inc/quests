package httpapi

import (
	"encoding/json"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/search"
	"net/http"
)

func (s *Server) searchEntities(w http.ResponseWriter, r *http.Request) {
	var q search.Request
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeErr(w, 400, "invalid search request")
		return
	}
	if err := q.Normalize(); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	if source := httpx.RequestSource(r); source != nil && *source == "mcp" {
		q.ExcludePrivate = true
	}
	page, err := s.Store.SearchEntities(r.Context(), q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, page)
}
