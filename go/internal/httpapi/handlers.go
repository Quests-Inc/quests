package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	automationhttp "github.com/quests-inc/quests/go/internal/automation/http"
	"github.com/quests-inc/quests/go/internal/execution"
	executionhttp "github.com/quests-inc/quests/go/internal/execution/http"
	"github.com/quests-inc/quests/go/internal/graphdraft"
	"github.com/quests-inc/quests/go/internal/identity"
	identityhttp "github.com/quests-inc/quests/go/internal/identity/http"
	journalhttp "github.com/quests-inc/quests/go/internal/journal/http"
	"github.com/quests-inc/quests/go/internal/kernel"
	knowledgehttp "github.com/quests-inc/quests/go/internal/knowledge/http"
	"github.com/quests-inc/quests/go/internal/platform/clamav"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/health"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/metrics"
	"github.com/quests-inc/quests/go/internal/platform/ratelimit"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
	"github.com/quests-inc/quests/go/internal/store"
)

type Server struct {
	// Version is the release this server was built from ("dev" locally); shown in /api/health.
	Version string
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
	Auth *identity.Store
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
	identityOnce     sync.Once
	knowledgeOnce    sync.Once
	journalOnce      sync.Once
	jh               *journalhttp.Handler
	automationOnce   sync.Once
	executionOnce    sync.Once
	graphdraftOnce   sync.Once
	gdh              *graphdraft.Handler
	ah               *automationhttp.Handler
	eh               *executionhttp.Handler
	kh               *knowledgehttp.Handler
	idh              *identityhttp.Handler
	loginLimiter     *ratelimit.Limiter
	// InternalToken authenticates the server's own loopback calls (LLM assistant).
	// Generated per process, never persisted.
	InternalToken string
	// SelfBase is this server's own loopback URL (http://127.0.0.1:PORT) —
	// used by the LLM action-batch assistant to call the same HTTP API the
	// frontend/CLI use, in-process, regardless of what host QUESTS_HOST binds.
	SelfBase string
	// RunnerDone is called once for every runner job that reaches a final state
	// (result delivered, lease lost, never picked up): the scheduler applies it to the step.
	RunnerDone func(ctx context.Context, j execution.RunnerJob)
	// WebDAV stores attachment bytes. Nil/unconfigured disables uploads.
	WebDAV *webdav.Client
	// ClamAV scans uploads. Unconfigured refuses uploads rather than storing
	// anything unscanned.
	ClamAV *clamav.Client
	// MaxUploadBytes caps a single attachment.
	MaxUploadBytes int64
	// WorkspaceQuota caps the attachment bytes of one workspace (0 = unlimited).
	WorkspaceQuota int64
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

// maxJSONBody caps a JSON request body; uploads (multipart) carry their own limit.
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
		if r.Body != nil && r.Body != http.NoBody &&
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
	mux.HandleFunc("GET /api/sync", s.getSync)
	mux.HandleFunc("GET /api/events", s.getEvents)
	mux.HandleFunc("POST /api/ui/focus-quest", s.postFocusQuest)
	mux.HandleFunc("GET /ws", s.wsEvents)
	s.identityHTTP().Register(mux)
	s.knowledgeHTTP().Register(mux)
	s.journalHTTP().Register(mux)
	s.registerPDF(mux)
	s.registerDomainRoutes(mux)
	mux.HandleFunc("POST /api/llm/actions/preview", func(w http.ResponseWriter, r *http.Request) { s.assistantHandler().Preview(w, r) })
	mux.HandleFunc("POST /api/llm/actions/apply", func(w http.ResponseWriter, r *http.Request) { s.assistantHandler().Apply(w, r) })
	mux.HandleFunc("GET /api/me/llm", func(w http.ResponseWriter, r *http.Request) { s.assistantHandler().GetMy(w, r) })
	mux.HandleFunc("PUT /api/me/llm", func(w http.ResponseWriter, r *http.Request) { s.assistantHandler().PutMy(w, r) })
	mux.HandleFunc("DELETE /api/me/llm", func(w http.ResponseWriter, r *http.Request) { s.assistantHandler().DeleteMy(w, r) })
	mux.HandleFunc("POST /api/me/llm/test", func(w http.ResponseWriter, r *http.Request) { s.assistantHandler().TestMy(w, r) })
	s.graphdraftHandler().Register(mux)
	mux.HandleFunc("GET /api/workspaces/{id}/export", func(w http.ResponseWriter, r *http.Request) { s.wsarchiveHandler().Export(w, r) })
	mux.HandleFunc("POST /api/workspaces/import", func(w http.ResponseWriter, r *http.Request) { s.wsarchiveHandler().Import(w, r) })
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

var writeJSON = httpx.WriteJSON

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}

func (s *Server) getHealth(w http.ResponseWriter, r *http.Request) {
	comps := s.Health.Snapshot()
	// The web shows status chips to everyone; the detail text (versions, errors, file names) stays
	// on the host: the admin CLI (`quests-server backup-now`, logs) has it.
	for name, c := range comps {
		if m, ok := c.(map[string]any); ok {
			m["detail"] = ""
			comps[name] = m
		}
	}
	overall := "ok"
	for _, c := range comps {
		m, _ := c.(map[string]any)
		st, _ := m["status"].(string)
		// warn (e.g. DB schema ahead of binary) still counts as degraded so the UI notices.
		if st != "ok" && st != "disabled" {
			overall = "degraded"
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":              overall,
		"api":                 map[string]string{"status": "ok", "version": s.Version},
		"components":          comps,
		"stale_after_seconds": health.StaleAfter,
	})
}

func (s *Server) postHeartbeat(w http.ResponseWriter, r *http.Request) {
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
