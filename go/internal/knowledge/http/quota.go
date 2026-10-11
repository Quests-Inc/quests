package knowledgehttp

import (
	"context"
	"fmt"
	"net/http"
)

// errQuota is what an upload that would not fit says.
func QuotaMessage(used, quota int64) string {
	return fmt.Sprintf("квота вложений пространства исчерпана: занято %d из %d МБ — удалите ненужные файлы или старые ревизии", used>>20, quota>>20)
}

// OverQuota reports whether adding incoming bytes to the context's workspace would pass its quota.
func (h *Handler) OverQuota(ctx context.Context, incoming int64) (bool, int64, error) {
	if h.Quota() <= 0 {
		return false, 0, nil
	}
	used, err := h.Store.AttachmentUsage(ctx)
	if err != nil {
		return false, 0, err
	}
	return used+incoming > h.Quota(), used, nil
}

// CheckQuota answers 413 itself and returns false when the upload does not fit.
func (h *Handler) CheckQuota(w http.ResponseWriter, ctx context.Context, incoming int64) bool {
	over, used, err := h.OverQuota(ctx, incoming)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if over {
		writeErr(w, http.StatusRequestEntityTooLarge, QuotaMessage(used, h.Quota()))
		return false
	}
	return true
}
