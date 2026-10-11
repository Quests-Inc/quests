package httpapi

import (
	"net/http"
	"strconv"
)

// listEmitAttempts returns the template's emit_pool_command execution log,
// newest first (?limit=, default 50, max 200). Messages are already free of
// template secret values — see schedule.execEmitPoolCommand.
func (s *Server) listEmitAttempts(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid template id")
		return
	}
	var one int
	if err := s.Store.DB.QueryRowContext(r.Context(), `SELECT 1 FROM questtemplate WHERE id = ?`, id).Scan(&one); err != nil {
		writeErr(w, http.StatusNotFound, "Template not found")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := s.Store.Automation().ListEmitAttempts(r.Context(), id, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempts": rows})
}
