// Package config loads/saves data/overlay.json (shared with the Python HUD).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config is the persisted HUD settings subset used by the Go overlay.
type Config struct {
	StylePack        string  `json:"style_pack"`
	MonitorIndex     int     `json:"monitor_index"`
	MonitorConnector string  `json:"monitor_connector"`
	MarginTop        int     `json:"margin_top"`
	MarginRight      int     `json:"margin_right"`
	PassthroughBG    string  `json:"passthrough_bg_mode"`
	PassthroughAlpha float64 `json:"passthrough_bg_alpha"`
	HUDTextAlpha     float64 `json:"hud_text_alpha"`
	ToastsMajor      bool    `json:"toasts_major"`
	ToastsMinorMode  string  `json:"toasts_minor_mode"`
	MinorBGMode      string  `json:"minor_bg_mode"`
	MinorBGAlpha     float64 `json:"minor_bg_alpha"`
	MinorTextAlpha   float64 `json:"minor_text_alpha"`
	MinorLogWidth    int     `json:"minor_log_width"`
	MinorLogHeight   int     `json:"minor_log_height"`
	HUDCategory      string  `json:"hud_category"`
	APIBase          string  `json:"api_base"`
}

func Defaults() Config {
	return Config{
		StylePack:        "fantasy",
		MonitorIndex:     0,
		MarginTop:        24,
		MarginRight:      24,
		PassthroughBG:    "chips",
		PassthroughAlpha: 0.6,
		HUDTextAlpha:     0.92,
		ToastsMajor:      true,
		ToastsMinorMode:  "toast",
		MinorBGMode:      "chips",
		MinorBGAlpha:     0.72,
		MinorTextAlpha:   0.92,
		MinorLogWidth:    520,
		MinorLogHeight:   280,
	}
}

// Path returns the overlay.json location.
func Path() string {
	if p := strings.TrimSpace(os.Getenv("QUESTS_OVERLAY_CONFIG")); p != "" {
		return p
	}
	data := strings.TrimSpace(os.Getenv("QUESTS_DATA_DIR"))
	if data == "" {
		data = resolveDataDir()
	}
	return filepath.Join(data, "overlay.json")
}

func resolveDataDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "data"
	}
	for _, cand := range []string{
		filepath.Join(wd, "data"),
		filepath.Join(wd, "..", "data"),
	} {
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand
		}
	}
	return filepath.Join(wd, "data")
}

// Load merges defaults with overlay.json (missing file → defaults).
func Load() Config {
	cfg := Defaults()
	raw, err := os.ReadFile(Path())
	if err != nil {
		applyEnv(&cfg)
		return cfg
	}
	var file Config
	if json.Unmarshal(raw, &file) != nil {
		applyEnv(&cfg)
		return cfg
	}
	merge(&cfg, file)
	applyEnv(&cfg)
	return cfg
}

// Save writes cfg to overlay.json (best-effort).
func Save(cfg Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(raw, '\n'), 0o644)
}

func applyEnv(cfg *Config) {
	if p := strings.TrimSpace(os.Getenv("QUESTS_STYLE_PACK")); p != "" {
		cfg.StylePack = p
	}
}

func merge(dst *Config, src Config) {
	if src.StylePack != "" {
		dst.StylePack = strings.TrimSpace(src.StylePack)
	}
	if src.MonitorIndex >= 0 {
		dst.MonitorIndex = src.MonitorIndex
	}
	dst.MonitorConnector = strings.TrimSpace(src.MonitorConnector)
	if src.MarginTop >= 0 {
		dst.MarginTop = src.MarginTop
	}
	if src.MarginRight >= 0 {
		dst.MarginRight = src.MarginRight
	}
	if src.PassthroughBG != "" {
		dst.PassthroughBG = normalizeBG(src.PassthroughBG)
	}
	dst.PassthroughAlpha = clampAlpha(src.PassthroughAlpha, dst.PassthroughAlpha)
	dst.HUDTextAlpha = clampAlpha(src.HUDTextAlpha, dst.HUDTextAlpha)
	dst.ToastsMajor = src.ToastsMajor
	if src.ToastsMinorMode != "" {
		dst.ToastsMinorMode = normalizeMinor(src.ToastsMinorMode)
	}
	if src.MinorBGMode != "" {
		dst.MinorBGMode = normalizeBG(src.MinorBGMode)
	}
	dst.MinorBGAlpha = clampAlpha(src.MinorBGAlpha, dst.MinorBGAlpha)
	dst.MinorTextAlpha = clampAlpha(src.MinorTextAlpha, dst.MinorTextAlpha)
	if src.MinorLogWidth > 0 {
		dst.MinorLogWidth = src.MinorLogWidth
	}
	if src.MinorLogHeight > 0 {
		dst.MinorLogHeight = src.MinorLogHeight
	}
	dst.HUDCategory = strings.TrimSpace(src.HUDCategory)
	dst.APIBase = strings.TrimRight(strings.TrimSpace(src.APIBase), "/")
}

func normalizeBG(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "full", "panel", "solid":
		return "full"
	default:
		return "chips"
	}
}

func normalizeMinor(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "toast", "log":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "toast"
	}
}

func clampAlpha(v, fallback float64) float64 {
	if v < 0 || v > 1 {
		if fallback >= 0 && fallback <= 1 {
			return fallback
		}
		return 0.6
	}
	return v
}
