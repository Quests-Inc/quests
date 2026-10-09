package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/valirum/quests/go/internal/wsctx"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valirum/quests/go/internal/auth"
	"github.com/valirum/quests/go/internal/clamav"
	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/health"
	"github.com/valirum/quests/go/internal/metrics"
	"github.com/valirum/quests/go/internal/refs"
	"github.com/valirum/quests/go/internal/secretbox"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/timeutil"
	"github.com/valirum/quests/go/internal/webdav"
)

type Server struct {
	// Metrics serves /metrics (nil = not built in); MetricsToken guards it (empty = disabled).
	Metrics      *metrics.Metrics
	MetricsToken string
	Store        *store.Store
	Health       *health.Registry
	Hub          *events.Hub
	CORS         []string
	DataDir      string
	Root         string
	// Auth backs login, sessions and API tokens.
	Auth *auth.Store
	// AuthRequired gates every /api/ route and /ws. Off only for a bootstrap
	// instance with no accounts yet, bound to loopback.
	AuthRequired bool
	// SecureCookies forces the session cookie Secure even when a request
	// can't be detected as HTTPS (see requestIsHTTPS). Per-request detection
	// already covers the gateway; this stays as a belt-and-suspenders switch.
	SecureCookies bool
	// loginLimiter throttles repeated failed /api/auth/login attempts.
	// Lazily initialized on first use — see Server.limiter().
	loginLimiterOnce sync.Once
	loginLimiter     *loginLimiter
	// InternalToken authenticates the server's own loopback calls (LLM assistant).
	// Generated per process, never persisted.
	InternalToken string
	// SelfBase is this server's own loopback URL (http://127.0.0.1:PORT) —
	// used by the LLM action-batch assistant to call the same HTTP API the
	// frontend/CLI use, in-process, regardless of what host QUESTS_HOST binds.
	SelfBase string
	// RunnerDone is called once for every runner job that reaches a final state
	// (result delivered, lease lost, never picked up): the scheduler applies it to the step.
	RunnerDone func(ctx context.Context, j store.RunnerJob)
	// WebDAV stores attachment bytes. Nil/unconfigured disables uploads.
	WebDAV *webdav.Client
	// ClamAV scans uploads. Unconfigured refuses uploads rather than storing
	// anything unscanned.
	ClamAV *clamav.Client
	// MaxUploadBytes caps a single attachment.
	MaxUploadBytes int64
	// Secrets seals what is stored for an account (its LLM key). Nil = nothing can be stored.
	Secrets *secretbox.Box
}

func (s *Server) Handler() http.Handler {
	h := s.secure(s.cors(s.requireAuth(s.withWorkspace(s.routes()))))
	if s.Metrics != nil {
		h = s.Metrics.Middleware(h)
	}
	return h
}

// maxJSONBody caps a JSON request body; uploads (multipart) and PDF export carry their own limits.
const maxJSONBody = 8 << 20

// contentSecurityPolicy is the shell's: scripts only from this origin (the build has no
// inline script), so markup that slips past sanitisation still cannot run code.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' data: https://fonts.gstatic.com; " +
	"img-src 'self' data: blob:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'none'; " +
	"frame-ancestors 'none'; form-action 'self'"

// secure sets browser hardening headers on every response and bounds JSON bodies.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		// Invitation links carry their secret in the URL: never hand it to another site.
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		if r.Body != nil && r.Body != http.NoBody && r.URL.Path != "/api/export/pdf" &&
			strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
		}
		next.ServeHTTP(w, r)
	})
}

// routes builds the API routes without the cors/auth wrappers; the draft
// commit uses it to call handlers directly.
func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	if s.Metrics != nil {
		mux.Handle("GET /metrics", s.Metrics.Handler(s.MetricsToken))
	}
	mux.HandleFunc("GET /api/health", s.getHealth)
	mux.HandleFunc("POST /api/health/heartbeat", s.postHeartbeat)
	mux.HandleFunc("GET /api/categories", s.listCategories)
	mux.HandleFunc("GET /api/sync", s.getSync)
	mux.HandleFunc("GET /api/events", s.getEvents)
	mux.HandleFunc("POST /api/ui/focus-quest", s.postFocusQuest)
	mux.HandleFunc("GET /ws", s.wsEvents)
	mux.HandleFunc("GET /api/quests", s.listQuests)
	mux.HandleFunc("POST /api/quests", s.createQuest)
	mux.HandleFunc("GET /api/quests/{id}", s.getQuest)
	mux.HandleFunc("PATCH /api/quests/{id}", s.patchQuest)
	mux.HandleFunc("DELETE /api/quests/{id}", s.deleteQuest)
	mux.HandleFunc("POST /api/quests/{id}/steps", s.addStep)
	mux.HandleFunc("PATCH /api/quests/{id}/steps/{step_id}", s.patchStep)
	mux.HandleFunc("DELETE /api/quests/{id}/steps/{step_id}", s.deleteStep)
	s.registerAuth(mux)
	s.registerAttachments(mux)
	s.registerNotes(mux)
	s.registerPDF(mux)
	s.registerParity(mux)
	s.registerTags(mux)
	s.registerLLMActions(mux)
	s.registerLLMSettings(mux)
	s.registerGraphDraft(mux)
	s.registerRunner(mux)
	s.registerWorkspaces(mux)
	s.registerWorkspaceArchive(mux)
	s.registerTokens(mux)
	s.mountSPA(mux)
	return mux
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range s.CORS {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			// Session cookie must survive the Vite dev origin.
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, detail string) {
	// A store refusal (command on the server in an untrusted workspace) is a 403, wherever it surfaced.
	if status >= 500 && (strings.Contains(detail, store.ErrServerExec.Error()) || strings.Contains(detail, store.ErrCommandOwner.Error())) {
		status = http.StatusForbidden
	}
	writeJSON(w, status, map[string]string{"detail": detail})
}

// isOperator reports whether the caller runs this server: an owner of a trusted workspace
// (the one allowed to run commands on the host), or no accounts at all. Server-wide facts
// (backup file names, schema version) and health heartbeats are theirs, not an invited member's.
func (s *Server) isOperator(r *http.Request) bool {
	p, ok := PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 || !s.Store.WorkspacesEnabled() {
		return true
	}
	ctx := r.Context()
	if !s.Store.ServerExecAllowed(ctx) {
		return false
	}
	role, err := s.Store.MemberRole(ctx, wsctx.From(ctx), p.User.ID)
	return err == nil && role == store.RoleOwner
}

func (s *Server) getHealth(w http.ResponseWriter, r *http.Request) {
	comps := s.Health.Snapshot()
	if !s.isOperator(r) {
		for name, c := range comps {
			if m, ok := c.(map[string]any); ok {
				m["detail"] = ""
				comps[name] = m
			}
		}
	}
	overall := "ok"
	for _, c := range comps {
		m, _ := c.(map[string]any)
		st, _ := m["status"].(string)
		// warn (e.g. DB alembic ahead of binary) still counts as degraded so the UI notices.
		if st != "ok" && st != "disabled" {
			overall = "degraded"
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":              overall,
		"api":                 map[string]string{"status": "ok"},
		"components":          comps,
		"stale_after_seconds": health.StaleAfter,
	})
}

func (s *Server) postHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !s.isOperator(r) {
		writeErr(w, http.StatusForbidden, "heartbeats are for the server's operator")
		return
	}
	var body struct {
		Component string `json:"component"`
		Detail    string `json:"detail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Component != "overlay" && body.Component != "telegram" {
		writeErr(w, http.StatusBadRequest, "component must be overlay or telegram")
		return
	}
	s.Health.Record(body.Component, body.Detail)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "component": body.Component})
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListCategories(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []domain.Category{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) listQuests(w http.ResponseWriter, r *http.Request) {
	var f store.ListFilter
	if st := r.URL.Query().Get("status"); st != "" {
		s := domain.QuestStatus(st)
		if !s.Valid() {
			writeErr(w, http.StatusUnprocessableEntity, domain.InvalidStatusMsg(st))
			return
		}
		f.Status = &s
	}
	if p := r.URL.Query().Get("pinned"); p != "" {
		v := p == "1" || strings.EqualFold(p, "true")
		f.Pinned = &v
	}
	if ql := r.URL.Query().Get("questline_id"); ql != "" {
		if n, err := strconv.ParseInt(ql, 10, 64); err == nil {
			f.QuestlineID = &n
		}
	}
	quests, err := s.Store.ListQuests(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := timeutil.NowUTC()
	out := make([]domain.QuestRead, 0, len(quests))
	for _, q := range quests {
		out = append(out, domain.ToQuestRead(q, now))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	q, err := s.Store.GetQuest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := domain.ToQuestRead(q, timeutil.NowUTC())
	out.Refs = s.resolveRefs(r, refs.Parse(q.Title+"\n"+q.Description))
	writeJSON(w, http.StatusOK, out)
}

// sourcePattern limits what a client may put into quest.source (a short label
// like "web", "cli", "mcp", "telegram"); "template:<id>" is reserved for the
// scheduler.
var sourcePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,31}$`)

// requestSource is how the quest being created was made, from the
// X-Quests-Source header the clients set; "api" when absent or not a plain label.
func requestSource(r *http.Request) *string {
	v := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Quests-Source")))
	if !sourcePattern.MatchString(v) {
		v = "api"
	}
	return &v
}

func (s *Server) createQuest(w http.ResponseWriter, r *http.Request) {
	var body domain.QuestCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		writeErr(w, http.StatusUnprocessableEntity, "title required")
		return
	}
	if body.Status == "" {
		body.Status = domain.StatusActive
	}
	if !body.Status.Valid() {
		writeErr(w, http.StatusUnprocessableEntity, domain.InvalidStatusMsg(string(body.Status)))
		return
	}
	if body.Significance == "" {
		body.Significance = domain.SigCommon
	}
	if !body.Significance.Valid() {
		writeErr(w, http.StatusUnprocessableEntity, domain.InvalidSignificanceMsg(string(body.Significance)))
		return
	}
	now := timeutil.NowUTC()
	var deadline *time.Time
	if body.DeadlineAt != nil && strings.TrimSpace(*body.DeadlineAt) != "" {
		t, err := timeutil.ParseFlexible(*body.DeadlineAt)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "invalid deadline_at")
			return
		}
		deadline = &t
	}
	d, dur := timeutil.NormalizeDeadline(deadline, body.DurationSeconds, body.DurationSeconds != nil, now)
	catID, err := store.ValidateCategoryID(r.Context(), s.Store, body.CategoryID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	lineID := body.QuestlineID
	if lineID != nil {
		ok, err := s.Store.QuestlineExists(r.Context(), *lineID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, "questline not found")
			return
		}
		if c, err := s.Store.QuestlineCategory(r.Context(), *lineID); err == nil && c != nil {
			catID = c
		}
	}
	q := domain.Quest{
		Title:           body.Title,
		Description:     body.Description,
		Status:          body.Status,
		Significance:    body.Significance,
		Pinned:          body.Pinned && !body.OverlayHidden,
		OverlayHidden:   body.OverlayHidden && !body.Pinned,
		SortOrder:       body.SortOrder,
		DeadlineAt:      d,
		DurationSeconds: dur,
		CategoryID:      catID,
		QuestlineID:     lineID,
		CreatedAt:       now,
		UpdatedAt:       now,
		Automated:       body.Automated,
		Source:          requestSource(r),
	}
	if len(body.Steps) == 0 {
		q.Steps = []domain.Step{{Title: body.Title, ProgressTotal: 1, SortOrder: 0}}
	} else {
		for i, sc := range body.Steps {
			ord := i
			if sc.SortOrder != nil {
				ord = *sc.SortOrder
			}
			cmd, iv := store.NormalizeCheck(sc.CheckCommand, sc.CheckIntervalSeconds)
			st := domain.Step{
				Title: sc.Title, Description: sc.Description,
				ProgressCurrent: sc.ProgressCurrent, ProgressTotal: sc.ProgressTotal,
				SortOrder: ord, CheckCommand: cmd, CheckIntervalSeconds: iv,
				RunOn: store.NormalizeRunOn(sc.RunOn),
			}
			if !store.ValidRunOn(st.RunOn) {
				writeErr(w, http.StatusUnprocessableEntity, "run_on: "+store.ErrBadRunnerLabel.Error())
				return
			}
			if sc.WaitPrevious != nil {
				st.WaitPrevious = *sc.WaitPrevious
			}
			if sc.RunMode != nil {
				st.RunMode = store.NormalizeRunMode(*sc.RunMode)
			}
			store.NormalizeStepCheck(&st)
			if st.ProgressTotal < 1 {
				st.ProgressTotal = 1
			}
			q.Steps = append(q.Steps, st)
		}
	}
	created, err := s.Store.CreateQuest(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.TagIDs != nil {
		if err := s.Store.SetQuestTags(r.Context(), created.ID, body.TagIDs); err != nil {
			writeTagErr(w, err)
			return
		}
		created, err = s.Store.GetQuest(r.Context(), created.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	quiet, source := s.publishOpts(r)
	qid := created.ID
	s.Hub.Publish("quest_created", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &qid, Title: created.Title, Description: created.Description,
		Detail: "создано задание", Toast: !quiet, Source: source,
		Significance: string(created.Significance),
		Automated:    created.Automated,
	})
	writeJSON(w, http.StatusCreated, domain.ToQuestRead(created, timeutil.NowUTC()))
}

func (s *Server) patchQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, ok := raw["steps"]; ok {
		writeErr(w, http.StatusUnprocessableEntity, "steps replace removed; use step CRUD")
		return
	}
	q, err := s.Store.GetQuest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	beforeStatus := q.Status
	deadlineTouched := false
	durationExplicit := false
	if v, ok := raw["title"]; ok {
		_ = json.Unmarshal(v, &q.Title)
	}
	if v, ok := raw["description"]; ok {
		_ = json.Unmarshal(v, &q.Description)
	}
	if v, ok := raw["status"]; ok {
		var st domain.QuestStatus
		_ = json.Unmarshal(v, &st)
		if !st.Valid() {
			writeErr(w, http.StatusUnprocessableEntity, domain.InvalidStatusMsg(string(st)))
			return
		}
		q.Status = st
	}
	if v, ok := raw["significance"]; ok {
		var sg domain.Significance
		_ = json.Unmarshal(v, &sg)
		if !sg.Valid() {
			writeErr(w, http.StatusUnprocessableEntity, domain.InvalidSignificanceMsg(string(sg)))
			return
		}
		q.Significance = sg
	}
	// pinned and overlay_hidden share one column, so turning one on turns the other off.
	if v, ok := raw["pinned"]; ok {
		_ = json.Unmarshal(v, &q.Pinned)
		if q.Pinned {
			q.OverlayHidden = false
		}
	}
	if v, ok := raw["overlay_hidden"]; ok {
		_ = json.Unmarshal(v, &q.OverlayHidden)
		if q.OverlayHidden {
			q.Pinned = false
		}
	}
	if v, ok := raw["sort_order"]; ok {
		_ = json.Unmarshal(v, &q.SortOrder)
	}
	if v, ok := raw["category_id"]; ok {
		var cid *int64
		_ = json.Unmarshal(v, &cid)
		cid, err = store.ValidateCategoryID(r.Context(), s.Store, cid)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		q.CategoryID = cid
	}
	if v, ok := raw["questline_id"]; ok {
		var lid *int64
		_ = json.Unmarshal(v, &lid)
		q.QuestlineID = lid
	}
	// A quest's category always follows its questline's category when one is
	// assigned — ignore a manually supplied category_id in that case, so the
	// two can never drift apart (regardless of which field this request touched).
	if q.QuestlineID != nil {
		if c, err := s.Store.QuestlineCategory(r.Context(), *q.QuestlineID); err == nil && c != nil {
			q.CategoryID = c
		}
	}
	if v, ok := raw["automated"]; ok {
		_ = json.Unmarshal(v, &q.Automated)
	}
	if v, ok := raw["deadline_at"]; ok {
		deadlineTouched = true
		if string(v) == "null" {
			q.DeadlineAt = nil
		} else {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				t, err := timeutil.ParseFlexible(s)
				if err != nil {
					writeErr(w, http.StatusUnprocessableEntity, "invalid deadline_at")
					return
				}
				q.DeadlineAt = &t
			}
		}
	}
	if v, ok := raw["duration_seconds"]; ok {
		durationExplicit = true
		if string(v) == "null" {
			q.DurationSeconds = nil
		} else {
			var n int
			_ = json.Unmarshal(v, &n)
			q.DurationSeconds = &n
		}
	}
	now := timeutil.NowUTC()
	q.UpdatedAt = now
	if q.Status == domain.StatusCompleted && q.CompletedAt == nil {
		q.CompletedAt = &now
	}
	if q.Status != domain.StatusCompleted {
		q.CompletedAt = nil
	}
	if deadlineTouched || durationExplicit || (q.DeadlineAt != nil && q.DurationSeconds == nil) {
		d, dur := timeutil.NormalizeDeadline(q.DeadlineAt, q.DurationSeconds, durationExplicit, now)
		q.DeadlineAt = d
		q.DurationSeconds = dur
	}
	kind := "quest_updated"
	detail := "изменено"
	toast := false
	if q.Status != beforeStatus {
		switch q.Status {
		case domain.StatusCompleted:
			kind, detail, toast = "quest_completed", "завершено", true
		case domain.StatusFailed:
			kind, detail, toast = "quest_failed", "провалено", true
		case domain.StatusExpired:
			kind, detail, toast = "quest_expired", "просрочено", true
		default:
			kind, detail, toast = "status_changed", string(beforeStatus)+" → "+string(q.Status), true
		}
	}
	updated, err := s.Store.UpdateQuest(r.Context(), q, kind, detail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v, ok := raw["tag_ids"]; ok {
		var ids []int64
		if err := json.Unmarshal(v, &ids); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid tag_ids")
			return
		}
		if err := s.Store.SetQuestTags(r.Context(), updated.ID, ids); err != nil {
			writeTagErr(w, err)
			return
		}
		updated, err = s.Store.GetQuest(r.Context(), updated.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	quiet, source := s.publishOpts(r)
	qid := updated.ID
	s.Hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &qid, Title: updated.Title, Description: updated.Description,
		Detail: detail, Toast: !quiet && toast, Source: source,
		Significance: string(updated.Significance),
		Automated:    updated.Automated,
	})
	writeJSON(w, http.StatusOK, domain.ToQuestRead(updated, timeutil.NowUTC()))
}

func (s *Server) deleteQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	q, err := s.Store.GetQuest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.purgeOwnerAttachments(r.Context(), ownerQuest, id)
	if err := s.Store.DeleteQuest(r.Context(), id, q.Title); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := s.publishOpts(r)
	s.Hub.Publish("quest_deleted", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: q.Title, Description: q.Description,
		Detail: "удалено", Toast: !quiet, Source: source,
		Automated: q.Automated,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addStep(w http.ResponseWriter, r *http.Request) {
	qid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body domain.StepCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	q, err := s.Store.GetQuest(r.Context(), qid)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ord := 0
	if body.SortOrder != nil {
		ord = *body.SortOrder
	} else if len(q.Steps) > 0 {
		for _, st := range q.Steps {
			if st.SortOrder >= ord {
				ord = st.SortOrder + 1
			}
		}
	}
	cmd, iv := store.NormalizeCheck(body.CheckCommand, body.CheckIntervalSeconds)
	st := domain.Step{
		QuestID: qid, Title: strings.TrimSpace(body.Title), Description: body.Description,
		ProgressCurrent: body.ProgressCurrent, ProgressTotal: body.ProgressTotal,
		SortOrder: ord, CheckCommand: cmd, CheckIntervalSeconds: iv,
		RunOn: store.NormalizeRunOn(body.RunOn),
	}
	if !store.ValidRunOn(st.RunOn) {
		writeErr(w, http.StatusUnprocessableEntity, "run_on: "+store.ErrBadRunnerLabel.Error())
		return
	}
	if body.WaitPrevious != nil {
		st.WaitPrevious = *body.WaitPrevious
	}
	if body.RunMode != nil {
		st.RunMode = store.NormalizeRunMode(*body.RunMode)
	}
	store.NormalizeStepCheck(&st)
	if st.Title == "" {
		writeErr(w, http.StatusUnprocessableEntity, "title required")
		return
	}
	if st.ProgressTotal < 1 {
		st.ProgressTotal = 1
	}
	q.Steps = append(q.Steps, st)
	now := timeutil.NowUTC()
	domain.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	updated, err := s.Store.AddStep(r.Context(), qid, st, q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := s.publishOpts(r)
	id := updated.ID
	_ = quiet
	s.Hub.Publish("quest_updated", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: updated.Title, Detail: "+шаги: 1",
		Toast: false, Source: source, Significance: string(updated.Significance),
		Automated: updated.Automated,
	})
	writeJSON(w, http.StatusCreated, domain.ToQuestRead(updated, timeutil.NowUTC()))
}

func (s *Server) patchStep(w http.ResponseWriter, r *http.Request) {
	qid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	sid, _ := strconv.ParseInt(r.PathValue("step_id"), 10, 64)
	var body domain.StepUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	q, err := s.Store.GetQuest(r.Context(), qid)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var st *domain.Step
	for i := range q.Steps {
		if q.Steps[i].ID == sid {
			st = &q.Steps[i]
			break
		}
	}
	if st == nil {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	wasDone := st.Done
	if body.Title != nil {
		st.Title = *body.Title
	}
	if body.Description != nil {
		st.Description = *body.Description
	}
	if body.ProgressCurrent != nil {
		st.ProgressCurrent = *body.ProgressCurrent
	}
	if body.ProgressTotal != nil {
		st.ProgressTotal = *body.ProgressTotal
	}
	if body.SortOrder != nil {
		st.SortOrder = *body.SortOrder
	}
	if body.CheckCommand != nil || body.CheckIntervalSeconds != nil {
		cmd := st.CheckCommand
		iv := st.CheckIntervalSeconds
		if body.CheckCommand != nil {
			cmd = body.CheckCommand
		}
		if body.CheckIntervalSeconds != nil {
			iv = body.CheckIntervalSeconds
		}
		st.CheckCommand, st.CheckIntervalSeconds = store.NormalizeCheck(cmd, iv)
	}
	if body.WaitPrevious != nil {
		st.WaitPrevious = *body.WaitPrevious
	}
	if body.RunMode != nil {
		st.RunMode = store.NormalizeRunMode(*body.RunMode)
	}
	if body.RunOn != nil {
		st.RunOn = store.NormalizeRunOn(body.RunOn) // "" clears
		if !store.ValidRunOn(st.RunOn) {
			writeErr(w, http.StatusUnprocessableEntity, "run_on: "+store.ErrBadRunnerLabel.Error())
			return
		}
	}
	store.NormalizeStepCheck(st)
	domain.ClampStep(st)
	now := timeutil.NowUTC()
	domain.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	kind := "quest_updated"
	detail := "изменено"
	if st.Done {
		kind = "step_completed"
		detail = st.Title
	} else if wasDone {
		kind = "step_reopened"
		detail = st.Title
	}
	updated, err := s.Store.UpdateStep(r.Context(), *st, q, kind, detail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := s.publishOpts(r)
	id := updated.ID
	toast := kind == "step_completed" || kind == "quest_completed"
	s.Hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: updated.Title, Detail: detail,
		Toast: !quiet && toast, Source: source, Significance: string(updated.Significance),
		StepTitle: st.Title, Automated: updated.Automated,
	})
	writeJSON(w, http.StatusOK, domain.ToQuestRead(updated, timeutil.NowUTC()))
}

func (s *Server) deleteStep(w http.ResponseWriter, r *http.Request) {
	qid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	sid, _ := strconv.ParseInt(r.PathValue("step_id"), 10, 64)
	q, err := s.Store.GetQuest(r.Context(), qid)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(q.Steps) <= 1 {
		writeErr(w, http.StatusBadRequest, "Cannot delete the last step")
		return
	}
	found := false
	var next []domain.Step
	for _, st := range q.Steps {
		if st.ID == sid {
			found = true
			continue
		}
		next = append(next, st)
	}
	if !found {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	q.Steps = next
	now := timeutil.NowUTC()
	domain.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	updated, err := s.Store.DeleteStep(r.Context(), qid, sid, q)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Step not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := s.publishOpts(r)
	id := updated.ID
	_ = quiet
	s.Hub.Publish("quest_updated", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: updated.Title, Detail: "−шаги: 1",
		Toast: false, Source: source, Significance: string(updated.Significance),
		Automated: updated.Automated,
	})
	writeJSON(w, http.StatusOK, domain.ToQuestRead(updated, timeutil.NowUTC()))
}
