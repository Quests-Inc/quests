package httpapi

import "net/http"

func (s *Server) seedStarter(w http.ResponseWriter, r *http.Request) {
	if !s.identityHTTP().RequireOwner(w, r, "добавлять примеры") {
		return
	}
	report, err := s.Store.SeedStarter(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, report)
}
func (s *Server) removeStarter(w http.ResponseWriter, r *http.Request) {
	if !s.identityHTTP().RequireOwner(w, r, "удалять примеры") {
		return
	}
	n, err := s.Store.RemoveStarter(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"trashed": n})
}
