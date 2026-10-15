//go:build quests_overlay

// Package stylepack ports Python overlay style packs (palette + CSS + timings).
package stylepack

import (
	"fmt"
	"strconv"
	"strings"
)

const DefaultID = "fantasy"

// RGB is an 8-bit color channel triple.
type RGB struct{ R, G, B int }

// Theme is the visual + timing contract for one pack.
type Theme struct {
	ID    string
	Label string

	FontDisplay string
	FontBody    string
	FontToast   string
	FontQuest   string // empty → FontDisplay

	BG     RGB
	Radius int

	FG, FGMuted, FGDim string
	Accent, AccentHot  string
	Title, Section     string
	Border             string
	OK, Warn, Danger   string
	Step               string // empty → FG

	SigCommon, SigUncommon, SigEpic, SigLegendary string

	LetterSpacingTitle string
	UppercaseSection   bool
	ChipTextShadow     bool

	TitlePT, SectionPT, QuestPT          int
	MajorTitlePT, MajorEyebrowPT         int
	MajorPadY, MajorPadX, MajorPadBottom int

	HUDPadY, HUDPadX, HUDMinWidth int

	MajorFadeInMS, MajorFadeOutMS, MajorHoldMS int
	MinorFadeInMS, MinorFadeOutMS, MinorHoldMS int

	ShowSignificance bool
	ShowMajorBody    bool
	ShowStepProgress bool

	HardEdges  bool // cyberpunk-like radius 0
	Scanlines  bool
	CyberRoles bool // cyan journal / red titles / yellow steps
}

// Look is runtime HUD look overrides from overlay.json (not pack-owned).
type Look struct {
	Mode   string // chips | full
	PanelA float64
	TextA  float64
}

func (t Theme) QuestFont() string {
	if strings.TrimSpace(t.FontQuest) != "" {
		return t.FontQuest
	}
	return t.FontDisplay
}

func (t Theme) StepColor() string {
	if strings.TrimSpace(t.Step) != "" {
		return t.Step
	}
	return t.FG
}

func (t Theme) withDefaults() Theme {
	if t.TitlePT == 0 {
		t.TitlePT = 13
	}
	if t.SectionPT == 0 {
		t.SectionPT = 11
	}
	if t.QuestPT == 0 {
		t.QuestPT = 11
	}
	if t.MajorTitlePT == 0 {
		t.MajorTitlePT = 28
	}
	if t.MajorEyebrowPT == 0 {
		t.MajorEyebrowPT = 14
	}
	if t.MajorPadY == 0 {
		t.MajorPadY = 18
	}
	if t.MajorPadX == 0 {
		t.MajorPadX = 28
	}
	if t.MajorPadBottom == 0 {
		t.MajorPadBottom = 20
	}
	if t.HUDPadY == 0 {
		t.HUDPadY = 12
	}
	if t.HUDPadX == 0 {
		t.HUDPadX = 14
	}
	if t.HUDMinWidth == 0 {
		t.HUDMinWidth = 280
	}
	if t.MajorFadeInMS == 0 {
		t.MajorFadeInMS = 400
	}
	if t.MajorFadeOutMS == 0 {
		t.MajorFadeOutMS = 4800
	}
	if t.MajorHoldMS == 0 {
		t.MajorHoldMS = 1200
	}
	if t.MinorFadeInMS == 0 {
		t.MinorFadeInMS = 240
	}
	if t.MinorFadeOutMS == 0 {
		t.MinorFadeOutMS = 360
	}
	if t.MinorHoldMS == 0 {
		t.MinorHoldMS = 3000
	}
	if t.LetterSpacingTitle == "" {
		t.LetterSpacingTitle = "0.04em"
	}
	return t
}

func rgbaRGB(c RGB, a float64) string {
	return fmt.Sprintf("rgba(%d, %d, %d, %.3f)", c.R, c.G, c.B, clamp01(a))
}

func hexAlpha(hex string, a float64) string {
	r, g, b := parseHex(hex)
	return fmt.Sprintf("rgba(%d, %d, %d, %.3f)", r, g, b, clamp01(a))
}

func parseHex(hex string) (r, g, b int) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return 200, 200, 200
	}
	rv, _ := strconv.ParseUint(h[0:2], 16, 8)
	gv, _ := strconv.ParseUint(h[2:4], 16, 8)
	bv, _ := strconv.ParseUint(h[4:6], 16, 8)
	return int(rv), int(gv), int(bv)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
