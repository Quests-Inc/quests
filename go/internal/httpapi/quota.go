package httpapi

import (
	"context"
	"fmt"
	"net/http"
)

// errQuota is what an upload that would not fit says.
func quotaMessage(used, quota int64) string {
	return fmt.Sprintf("квота вложений пространства исчерпана: занято %d из %d МБ — удалите ненужные файлы или старые ревизии", used>>20, quota>>20)
}

// overQuota reports whether adding incoming bytes to the context's workspace would pass its quota.
func (s *Server) overQuota(ctx context.Context, incoming int64) (bool, int64, error) {
	if s.WorkspaceQuota <= 0 {
		return false, 0, nil
	}
	used, err := s.Store.AttachmentUsage(ctx)
	if err != nil {
		return false, 0, err
	}
	return used+incoming > s.WorkspaceQuota, used, nil
}

// checkQuota answers 413 itself and returns false when the upload does not fit.
func (s *Server) checkQuota(w http.ResponseWriter, ctx context.Context, incoming int64) bool {
	over, used, err := s.overQuota(ctx, incoming)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if over {
		writeErr(w, http.StatusRequestEntityTooLarge, quotaMessage(used, s.WorkspaceQuota))
		return false
	}
	return true
}
