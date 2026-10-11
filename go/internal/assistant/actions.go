package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/llmassist"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// Preview turns free text into a dry-run batch of actions.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text      string `json:"text"`
		NoClarify bool   `json:"no_clarify"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	if len([]rune(text)) > maxAssistantText {
		writeErr(w, http.StatusRequestEntityTooLarge, "текст длиннее "+strconv.Itoa(maxAssistantText)+" символов")
		return
	}
	settings, _, serr := h.llmSettingsFor(r.Context(), r)
	if serr != nil {
		writeErr(w, http.StatusConflict, serr.Error())
		return
	}
	if p, ok := identity.PrincipalFrom(r.Context()); ok && !h.TryLLM(strconv.FormatInt(p.User.ID, 10)) {
		writeErr(w, http.StatusTooManyRequests, "слишком много запросов к ассистенту, повторите позже")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	cats, err := h.Store.Journal.ListCategories(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	hints := make([]llmassist.CategoryHint, 0, len(cats))
	for _, c := range cats {
		hints = append(hints, llmassist.CategoryHint{ID: c.ID, Slug: c.Slug, Label: c.Label})
	}
	pc := llmassist.DefaultPromptContext(hints)
	pc.NoClarify = body.NoClarify

	batch, err := llmassist.ExtractActionBatch(ctx, settings, text, pc)
	if err != nil {
		// "detail" (not "error") — that's the key api.js's request() reads
		// for non-2xx bodies; otherwise the UI shows the bare HTTP
		// statusText ("Bad Gateway") instead of the real reason.
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if batch.NeedsClarification && body.NoClarify {
		// The model asked again despite the ban: don't pass a second question
		// on, report an empty plan (the client asks to rephrase).
		batch = llmassist.ActionBatch{}
	}
	if batch.NeedsClarification {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "needs_clarification": true, "clarify_question": batch.ClarifyQuestion,
		})
		return
	}

	executor := llmassist.NewExecutor(h.SelfBase, h.InternalToken, wsctx.From(r.Context()))
	preview, err := executor.Run(batch, true)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "needs_clarification": false, "batch": batch, "preview": preview,
	})
}

// Apply runs a confirmed batch.
func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Batch llmassist.ActionBatch `json:"batch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(body.Batch.Actions) == 0 {
		writeErr(w, http.StatusBadRequest, "batch is required")
		return
	}

	executor := llmassist.NewExecutor(h.SelfBase, h.InternalToken, wsctx.From(r.Context()))
	results, err := executor.Run(body.Batch, false)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "detail": err.Error(), "results": results})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": results})
}
