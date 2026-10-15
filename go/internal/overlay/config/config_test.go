package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAndNormalize(t *testing.T) {
	d := Defaults()
	if d.StylePack != "fantasy" || d.PassthroughBG != "chips" || !d.ToastsMajor {
		t.Fatalf("unexpected defaults: %+v", d)
	}
	if normalizeBG("FULL") != "full" || normalizeBG("x") != "chips" {
		t.Fatal("normalizeBG")
	}
	if normalizeMinor("LOG") != "log" || normalizeMinor("nope") != "toast" {
		t.Fatal("normalizeMinor")
	}
	if clampAlpha(0.3, 0.9) != 0.3 || clampAlpha(2, 0.4) != 0.4 || clampAlpha(-1, 9) != 0.6 {
		t.Fatal("clampAlpha")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.json")
	t.Setenv("QUESTS_OVERLAY_CONFIG", path)
	t.Setenv("QUESTS_STYLE_PACK", "nord")

	cfg := Load()
	if cfg.StylePack != "nord" {
		t.Fatalf("env override: %q", cfg.StylePack)
	}
	cfg.StylePack = "quiet"
	cfg.PassthroughBG = "panel"
	cfg.ToastsMinorMode = "log"
	cfg.MinorBGMode = "full"
	cfg.APIBase = "http://example:8765/"
	cfg.HUDCategory = " work "
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUESTS_STYLE_PACK", "")
	got := Load()
	if got.StylePack != "quiet" || got.PassthroughBG != "full" || got.ToastsMinorMode != "log" {
		t.Fatalf("round-trip: %+v", got)
	}
	if got.APIBase != "http://example:8765" || got.HUDCategory != "work" {
		t.Fatalf("trimmed fields: %+v", got)
	}
	if Path() != path {
		t.Fatalf("Path=%q", Path())
	}
}

func TestLoadMissingAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUESTS_OVERLAY_CONFIG", filepath.Join(dir, "missing.json"))
	cfg := Load()
	if cfg.StylePack != "fantasy" {
		t.Fatalf("missing file defaults: %+v", cfg)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUESTS_OVERLAY_CONFIG", bad)
	cfg = Load()
	if cfg.MarginTop != 24 {
		t.Fatalf("corrupt file defaults: %+v", cfg)
	}
}

func TestPathFromDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUESTS_OVERLAY_CONFIG", "")
	t.Setenv("QUESTS_DATA_DIR", dir)
	want := filepath.Join(dir, "overlay.json")
	if Path() != want {
		t.Fatalf("got %q want %q", Path(), want)
	}
}
