//go:build quests_overlay

package hud

import (
	"fmt"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
	"github.com/quests-inc/quests/go/internal/overlay/stylepack"
)

// settingsGroup is one keyboard-focusable row in the settings panel.
type settingsGroup struct {
	ID       string // monitor|style|category|bg_mode|…
	Label    string
	Kind     string // choice|scale
	Options  []opt  // choice
	Active   string // choice id
	Value    float64
	Min, Max float64
	Step     float64
	Suffix   string // "%" | "px" | ""
}

func buildSettingsGroups(cfg config.Config, cats []api.Category) []settingsGroup {
	out := make([]settingsGroup, 0, 16)

	mons := shell.ListMonitors(gdk.DisplayGetDefault())
	if len(mons) > 0 {
		opts := make([]opt, 0, len(mons))
		for i, mon := range mons {
			label := shell.MonitorConnector(mon)
			if label == "" {
				label = fmt.Sprintf("#%d", i+1)
			}
			opts = append(opts, opt{ID: strconv.Itoa(i), Label: label})
		}
		active := strconv.Itoa(shell.ResolveMonitorIndex(mons, cfg.MonitorConnector, cfg.MonitorIndex))
		out = append(out, settingsGroup{ID: "monitor", Label: "Монитор", Kind: "choice", Options: opts, Active: active})
	}

	packs := stylepack.List()
	if len(packs) > 0 {
		opts := make([]opt, 0, len(packs))
		for _, p := range packs {
			opts = append(opts, opt{ID: p.ID, Label: p.Label})
		}
		out = append(out, settingsGroup{
			ID: "style", Label: "Стиль", Kind: "choice", Options: opts, Active: stylepack.ActiveID(),
		})
	}

	catOpts := make([]opt, 0, len(cats))
	for _, c := range cats {
		if c.Slug == "" {
			continue
		}
		label := c.Label
		if label == "" {
			label = c.Slug
		}
		catOpts = append(catOpts, opt{ID: c.Slug, Label: label})
	}
	if len(catOpts) > 0 {
		active := cfg.HUDCategory
		if active == "" {
			active = catOpts[0].ID
		}
		out = append(out, settingsGroup{ID: "category", Label: "Раздел", Kind: "choice", Options: catOpts, Active: active})
	}

	bgMode := cfg.PassthroughBG
	if bgMode != "full" {
		bgMode = "chips"
	}
	out = append(out,
		settingsGroup{ID: "bg_mode", Label: "Фон", Kind: "choice", Options: []opt{{"chips", "выделение"}, {"full", "полный"}}, Active: bgMode},
		settingsGroup{ID: "bg_alpha", Label: "Альфа фона", Kind: "scale", Value: cfg.PassthroughAlpha * 100, Min: 0, Max: 100, Step: 5, Suffix: "%"},
		settingsGroup{ID: "text_alpha", Label: "Альфа текста", Kind: "scale", Value: cfg.HUDTextAlpha * 100, Min: 0, Max: 100, Step: 5, Suffix: "%"},
	)

	maj := "0"
	if cfg.ToastsMajor {
		maj = "1"
	}
	out = append(out, settingsGroup{ID: "major", Label: "Мажорные тосты", Kind: "choice", Options: []opt{{"1", "вкл"}, {"0", "выкл"}}, Active: maj})

	minorMode := cfg.ToastsMinorMode
	if minorMode != "off" && minorMode != "toast" && minorMode != "log" {
		minorMode = "toast"
	}
	mBG := cfg.MinorBGMode
	if mBG != "full" {
		mBG = "chips"
	}
	out = append(out,
		settingsGroup{ID: "minor_mode", Label: "Минор: режим", Kind: "choice", Options: []opt{{"off", "выкл"}, {"toast", "тост"}, {"log", "лог"}}, Active: minorMode},
		settingsGroup{ID: "minor_bg", Label: "Минор: выделение", Kind: "choice", Options: []opt{{"chips", "выделение"}, {"full", "полный"}}, Active: mBG},
		settingsGroup{ID: "minor_bg_a", Label: "Минор: альфа фона", Kind: "scale", Value: cfg.MinorBGAlpha * 100, Min: 0, Max: 100, Step: 5, Suffix: "%"},
		settingsGroup{ID: "minor_text_a", Label: "Минор: альфа текста", Kind: "scale", Value: cfg.MinorTextAlpha * 100, Min: 0, Max: 100, Step: 5, Suffix: "%"},
		settingsGroup{ID: "minor_w", Label: "Ширина лога", Kind: "scale", Value: float64(cfg.MinorLogWidth), Min: 280, Max: 900, Step: 20, Suffix: "px"},
		settingsGroup{ID: "minor_h", Label: "Высота лога", Kind: "scale", Value: float64(cfg.MinorLogHeight), Min: 120, Max: 800, Step: 20, Suffix: "px"},
	)
	return out
}

func choiceIndex(opts []opt, active string) int {
	for i, o := range opts {
		if o.ID == active {
			return i
		}
	}
	return 0
}

func cycleChoice(opts []opt, active string, delta int) string {
	if len(opts) == 0 {
		return active
	}
	i := choiceIndex(opts, active)
	n := len(opts)
	i = (i + delta) % n
	if i < 0 {
		i += n
	}
	return opts[i].ID
}

func clampScale(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
