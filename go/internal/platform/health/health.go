package health

import (
	"sync"
	"time"
)

const StaleAfter = 20.0

type Registry struct {
	mu   sync.Mutex
	seen map[string]heartbeat
}

type heartbeat struct {
	at          time.Time
	detail      string
	probeStatus string // set for services we ping ourselves (webdav, clamav)
}

func New() *Registry {
	return &Registry{seen: map[string]heartbeat{}}
}

func (r *Registry) Record(component, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen[component] = heartbeat{at: time.Now().UTC(), detail: detail}
}

// SetProbe records a status we observed by pinging, not a client heartbeat.
// status is ok | offline | disabled | warn.
func (r *Registry) SetProbe(component, status, detail string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen[component] = heartbeat{at: time.Now().UTC(), detail: detail, probeStatus: status}
}

func (r *Registry) Snapshot() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	out := map[string]any{}
	for _, name := range []string{"overlay", "telegram"} {
		out[name] = heartbeatView(r.seen[name], now, true)
	}
	for _, name := range []string{"webdav", "clamav", "backup", "db"} {
		hb, ok := r.seen[name]
		if !ok {
			out[name] = map[string]any{
				"status":       "disabled",
				"age_seconds":  nil,
				"last_seen_at": nil,
				"detail":       "",
			}
			continue
		}
		out[name] = heartbeatView(hb, now, false)
	}
	return out
}

func heartbeatView(hb heartbeat, now time.Time, staleOffline bool) map[string]any {
	comp := map[string]any{
		"status":       "offline",
		"age_seconds":  nil,
		"last_seen_at": nil,
		"detail":       "",
	}
	if hb.at.IsZero() && hb.probeStatus == "" && hb.detail == "" {
		return comp
	}
	if !hb.at.IsZero() {
		age := now.Sub(hb.at).Seconds()
		comp["detail"] = hb.detail
		comp["age_seconds"] = float64(int(age*10)) / 10
		comp["last_seen_at"] = hb.at.Format("2006-01-02T15:04:05Z")
		if hb.probeStatus != "" {
			comp["status"] = hb.probeStatus
		} else if staleOffline && age <= StaleAfter {
			comp["status"] = "ok"
		}
	} else if hb.probeStatus != "" {
		comp["status"] = hb.probeStatus
		comp["detail"] = hb.detail
	}
	return comp
}
