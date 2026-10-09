// Package metrics exposes Prometheus metrics (quest=345). Admin-only: the endpoint is
// guarded by its own bearer token and carries aggregates only, never workspace identity.
package metrics

import (
	"bufio"
	"context"
	"crypto/subtle"
	"database/sql"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/health"
	"github.com/valirum/quests/go/internal/store"
)

type Metrics struct {
	reg      *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inflight prometheus.Gauge
	app      *appCollector
}

// New builds the registry. db may be nil (no connection-pool metrics).
func New(db *sql.DB, st *store.Store, hub *events.Hub, hr *health.Registry) *Metrics {
	m := &Metrics{reg: prometheus.NewRegistry()}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "quests_http_requests_total", Help: "HTTP requests by route pattern and status."},
		[]string{"method", "route", "code"})
	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "quests_http_request_duration_seconds", Help: "HTTP request latency by route pattern.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}},
		[]string{"method", "route"})
	m.inflight = prometheus.NewGauge(prometheus.GaugeOpts{Name: "quests_http_requests_in_flight", Help: "Requests being served."})
	m.app = &appCollector{st: st, hub: hub, health: hr}
	m.reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests, m.duration, m.inflight, m.app)
	if db != nil {
		m.reg.MustRegister(collectors.NewDBStatsCollector(db, "quests"))
	}
	return m
}

// respWriter remembers the status code and keeps WebSocket upgrades (Hijack) and streaming (Flush) working.
type respWriter struct {
	http.ResponseWriter
	code int
}

func (w *respWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }

func (w *respWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	w.code = http.StatusSwitchingProtocols
	return h.Hijack()
}

func (w *respWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *respWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Middleware records request counts and latency, labelled by the matched route pattern
// (not the raw path: ids would explode the label space).
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inflight.Inc()
		defer m.inflight.Dec()
		start := time.Now()
		sw := &respWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		m.requests.WithLabelValues(r.Method, route, strconv.Itoa(sw.code)).Inc()
		m.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// Handler serves /metrics for a caller presenting the bearer token. An empty token disables it (404).
func (m *Metrics) Handler(token string) http.Handler {
	inner := promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.NotFound(w, r)
			return
		}
		got := r.Header.Get("Authorization")
		want := "Bearer " + token
		if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// appCollector reports application-level gauges. The database aggregates are cached for
// a short while: a scrape every 15 s must not turn into a query per workspace per scrape.
type appCollector struct {
	st     *store.Store
	hub    *events.Hub
	health *health.Registry

	mu     sync.Mutex
	at     time.Time
	cached store.Stats
	ok     bool
}

var (
	dQuests    = prometheus.NewDesc("quests_quests", "Quests by status, all workspaces.", []string{"status"}, nil)
	dNotes     = prometheus.NewDesc("quests_notes", "Notes, all workspaces.", nil, nil)
	dWorkspace = prometheus.NewDesc("quests_workspaces", "Live workspaces.", nil, nil)
	dJobs      = prometheus.NewDesc("quests_runner_jobs", "Runner jobs by state.", []string{"state"}, nil)
	dRunners   = prometheus.NewDesc("quests_runners", "Registered, not revoked runners.", nil, nil)
	dAttach    = prometheus.NewDesc("quests_attachments", "Attachments.", nil, nil)
	dAttBytes  = prometheus.NewDesc("quests_attachment_bytes", "Attachment bytes (current revisions).", nil, nil)
	dAttMax    = prometheus.NewDesc("quests_workspace_attachment_bytes_max", "Attachment bytes (all revisions) of the largest workspace.", nil, nil)
	dBackupAt  = prometheus.NewDesc("quests_backup_last_success_timestamp_seconds", "Newest successful DB backup.", nil, nil)
	dBackupSz  = prometheus.NewDesc("quests_backup_last_size_bytes", "Size of the newest successful DB backup.", nil, nil)
	dWS        = prometheus.NewDesc("quests_ws_clients", "Connected WebSocket clients.", nil, nil)
	dProbe     = prometheus.NewDesc("quests_component_up", "Component health probe (1 = ok).", []string{"component"}, nil)
	dStatsOK   = prometheus.NewDesc("quests_stats_collect_ok", "1 if the last database aggregate query succeeded.", nil, nil)
)

func (c *appCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{dQuests, dNotes, dWorkspace, dJobs, dRunners, dAttach, dAttBytes, dAttMax, dBackupAt, dBackupSz, dWS, dProbe, dStatsOK} {
		ch <- d
	}
}

func (c *appCollector) stats() (store.Stats, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.at) < 30*time.Second {
		return c.cached, c.ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := c.st.AggregateStats(ctx)
	c.at, c.ok = time.Now(), err == nil
	if err == nil || len(s.QuestsByStatus) > 0 {
		c.cached = s
	}
	return c.cached, c.ok
}

func (c *appCollector) Collect(ch chan<- prometheus.Metric) {
	gauge := func(d *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
	}
	s, ok := c.stats()
	if ok {
		gauge(dStatsOK, 1)
	} else {
		gauge(dStatsOK, 0)
	}
	for k, v := range s.QuestsByStatus {
		gauge(dQuests, float64(v), k)
	}
	for k, v := range s.RunnerJobsState {
		gauge(dJobs, float64(v), k)
	}
	gauge(dNotes, float64(s.Notes))
	gauge(dWorkspace, float64(s.Workspaces))
	gauge(dRunners, float64(s.RunnersActive))
	gauge(dAttach, float64(s.Attachments))
	gauge(dAttBytes, float64(s.AttachmentBytes))
	gauge(dAttMax, float64(s.AttachmentBytesMax))
	gauge(dBackupAt, float64(s.LastBackupAt))
	gauge(dBackupSz, float64(s.LastBackupBytes))
	if c.hub != nil {
		gauge(dWS, float64(c.hub.ClientCount()))
	}
	if c.health != nil {
		for name, v := range c.health.Snapshot() {
			m, _ := v.(map[string]any)
			status, _ := m["status"].(string)
			switch status {
			case "disabled":
				continue // not configured: neither up nor down
			case "ok":
				gauge(dProbe, 1, name)
			default:
				gauge(dProbe, 0, name)
			}
		}
	}
}
