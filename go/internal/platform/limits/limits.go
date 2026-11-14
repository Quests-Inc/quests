// Package limits holds administrator-only operational limits. The API has no write endpoint.
// Settings are loaded from a server-owned file; overrides take effect in the running process.
package limits

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Spec struct {
	Default  int64  `json:"default"`
	Min      int64  `json:"min"`
	Max      int64  `json:"max"`
	Unit     string `json:"unit"`
	Env      string `json:"env,omitempty"`
	EnvScale int64  `json:"-"`
}

var Specs = map[string]Spec{
	"egress_body":            {1 << 20, 1024, 64 << 20, "bytes", "QUESTS_EGRESS_MAX_BODY", 1},
	"egress_timeout":         {10, 1, 120, "seconds", "QUESTS_EGRESS_TIMEOUT", 1},
	"egress_rate":            {120, 1, 100000, "requests/hour", "QUESTS_EGRESS_RATE", 1},
	"egress_repository_rate": {500, 1, 100000, "requests/hour", "QUESTS_EGRESS_REPOSITORY_RATE", 1},
	"egress_concurrency":     {4, 1, 64, "requests", "QUESTS_EGRESS_CONCURRENCY", 1},
	"git_bytes":              {1 << 30, 1 << 20, 1 << 40, "bytes", "", 1},
	"attachment_bytes":       {2 << 30, 0, 1 << 40, "bytes", "QUESTS_WORKSPACE_QUOTA_MB", 1 << 20},
	"upload_bytes":           {25 << 20, 1 << 10, 1 << 30, "bytes", "QUESTS_MAX_UPLOAD_MB", 1 << 20},
	"values_own":             {32, 1, 1024, "entries", "", 1},
	"values_bytes":           {8 << 10, 1024, 1 << 20, "bytes", "", 1},
	"values_effective":       {64, 1, 4096, "names", "", 1},
	"values_chain":           {32, 1, 256, "notes", "", 1},
	"embed_auto":             {16, 1, 256, "inserts", "", 1},
	"foreign_values":         {8, 1, 512, "references", "", 1},
	"graph_nodes":            {200, 1, 2000, "nodes", "", 1},
	"build_bytes":            {64 << 10, 1024, 1 << 20, "bytes", "", 1},
}

type Row struct {
	RestartRequired bool `json:"restart_required"`
	Spec
	Value  int64  `json:"value"`
	Source string `json:"source"`
}

var mu sync.RWMutex
var filename string

func Configure(dataDir string) {
	mu.Lock()
	if dataDir == "" {
		filename = ""
	} else {
		filename = filepath.Join(dataDir, "server-limits.json")
	}
	mu.Unlock()
}
func Path() string { mu.RLock(); defer mu.RUnlock(); return filename }
func read(path string) (map[string]int64, error) {
	out := map[string]int64{}
	if path == "" {
		return out, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if err := validate(out); err != nil {
		return nil, err
	}
	return out, nil
}
func validate(values map[string]int64) error {
	for k, v := range values {
		s, ok := Specs[k]
		if !ok || v < s.Min || v > s.Max {
			return fmt.Errorf("%s: вне допустимого диапазона", k)
		}
	}
	own := Specs["values_own"].Default
	if v, ok := values["values_own"]; ok {
		own = v
	}
	effective := Specs["values_effective"].Default
	if v, ok := values["values_effective"]; ok {
		effective = v
	}
	if effective < own {
		return fmt.Errorf("values_effective должен быть >= values_own")
	}
	return nil
}
func List() (map[string]Row, error) {
	saved, err := read(Path())
	if err != nil {
		return nil, err
	}
	rows := map[string]Row{}
	for k, s := range Specs {
		r := Row{Spec: s, Value: s.Default, Source: "default", RestartRequired: strings.HasPrefix(k, "egress_")}
		if v, ok := saved[k]; ok {
			r.Value = v
			r.Source = "saved"
		}
		if s.Env != "" {
			if raw := strings.TrimSpace(os.Getenv(s.Env)); raw != "" {
				v, err := strconv.ParseInt(raw, 10, 64)
				if err != nil || v < 0 || v > s.Max/s.EnvScale || v*s.EnvScale < s.Min {
					return nil, fmt.Errorf("%s: invalid value", s.Env)
				}
				r.Value = v * s.EnvScale
				r.Source = "env"
			}
		}
		rows[k] = r
	}
	return rows, nil
}
func Get(key string) int64 {
	rows, err := List()
	if err != nil { // Invalid configuration must not silently fall back to a larger quota.
		return Specs[key].Min
	}
	return rows[key].Value
}

// Change is called only by the host CLI. Env overrides remain authoritative and are shown by List.
func Change(key string, value *int64) error {
	mu.Lock()
	defer mu.Unlock()
	if filename == "" {
		return fmt.Errorf("limits path not configured")
	}
	if _, ok := Specs[key]; !ok {
		return fmt.Errorf("unknown limit %q", key)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filename+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	values, err := read(filename)
	if err != nil {
		return err
	}
	old, had := values[key]
	if value == nil {
		delete(values, key)
	} else {
		values[key] = *value
	}
	if err := validate(values); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".server-limits-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Prepare audit before publishing a change, so a logging failure refuses the edit.
	log, err := os.OpenFile(filename+".audit.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	record := map[string]any{"at": time.Now().UTC(), "uid": os.Getuid(), "key": key, "previous": old, "was_set": had, "value": value}
	err = json.NewEncoder(log).Encode(record)
	if err == nil {
		err = log.Sync()
	}
	log.Close()
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}

// Freeze records startup values of limits held by existing HTTP transports/semaphores.
var startup map[string]int64

func Freeze() {
	rows, err := List()
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	startup = map[string]int64{}
	for k, v := range rows {
		if v.RestartRequired {
			startup[k] = v.Value
		}
	}
}
func Applied(key string) int64 {
	mu.RLock()
	v, ok := startup[key]
	mu.RUnlock()
	if ok {
		return v
	}
	return Get(key)
}

// EgressEnv preserves the host's address policy and supplies persisted numerical budgets.
func EgressEnv(name string) string {
	for key, s := range Specs {
		if strings.HasPrefix(key, "egress_") && s.Env == name {
			return strconv.FormatInt(Applied(key), 10)
		}
	}
	return os.Getenv(name)
}
