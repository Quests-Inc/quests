//go:build quests_overlay

package stylepack

import (
	"sort"
	"strings"
)

const (
	mono     = `"JetBrainsMono Nerd Font", "JetBrains Mono", "JetBrainsMono NF", monospace`
	sans     = `"IBM Plex Sans", "Noto Sans", "DejaVu Sans", sans-serif`
	serif    = `"Noto Serif", "Liberation Serif", "DejaVu Serif", serif`
	plexMono = `"IBM Plex Mono", "JetBrains Mono", ui-monospace, monospace`
)

var registry = map[string]Theme{}

func init() {
	for _, t := range []Theme{
		fantasyTheme(),
		cyberpunkTheme(),
		gruvboxTheme(),
		nordTheme(),
		roseTheme(),
		emberTheme(),
		inkTheme(),
		oceanTheme(),
		quietTheme(),
	} {
		registry[t.ID] = t.withDefaults()
	}
}

// Info is a short pack listing row.
type Info struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// List returns packs sorted by id.
func List() []Info {
	out := make([]Info, 0, len(registry))
	for _, t := range registry {
		out = append(out, Info{ID: t.ID, Label: t.Label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Resolve picks a pack by id (fallback fantasy).
func Resolve(id string) Theme {
	key := strings.ToLower(strings.TrimSpace(id))
	if t, ok := registry[key]; ok {
		return t
	}
	return registry[DefaultID]
}

// IDs returns known pack ids.
func IDs() []string {
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func fantasyTheme() Theme {
	return Theme{
		ID: "fantasy", Label: "Fantasy Journal",
		FontDisplay: mono, FontBody: mono, FontToast: serif,
		BG: RGB{26, 21, 16}, Radius: 8,
		FG: "#e8d5b0", FGMuted: "#c4a574", FGDim: "#8a7355",
		Accent: "#f0c86a", AccentHot: "#c4a574",
		Title: "#e8d5b0", Section: "#f0c86a", Border: "#c4a574",
		OK: "#8ec07c", Warn: "#fe8019", Danger: "#fb4934",
		SigCommon: "#a8a8a8", SigUncommon: "#8ec07c", SigEpic: "#d3869b", SigLegendary: "#fe8019",
		LetterSpacingTitle: "1px", UppercaseSection: false, ChipTextShadow: true,
		MajorTitlePT: 32, MajorEyebrowPT: 16,
		MajorFadeInMS: 500, MajorFadeOutMS: 5000, MajorHoldMS: 1200,
		MinorFadeInMS: 280, MinorFadeOutMS: 400, MinorHoldMS: 3200,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
	}
}

func cyberpunkTheme() Theme {
	tech := `"Rajdhani", "Oxanium", "Orbitron", "Noto Sans Display", "Noto Sans", "DejaVu Sans", sans-serif`
	return Theme{
		ID: "cyberpunk", Label: "Cyberpunk 2077",
		FontDisplay: tech, FontBody: tech, FontToast: serif,
		BG: RGB{12, 12, 14}, Radius: 0,
		FG: "#fcee0a", FGMuted: "#c4b808", FGDim: "#7a7400",
		Accent: "#00f0ff", AccentHot: "#00b8c4",
		Title: "#e03131", Section: "#00f0ff", Border: "#fcee0a",
		OK: "#3dff9a", Warn: "#ff8a1f", Danger: "#ff003c",
		Step:      "#fcee0a",
		SigCommon: "#a8a8a8", SigUncommon: "#3dff9a", SigEpic: "#c77dff", SigLegendary: "#ff8a1f",
		LetterSpacingTitle: "0.08em", UppercaseSection: true, ChipTextShadow: true,
		MajorTitlePT: 36, MajorEyebrowPT: 18,
		MajorFadeInMS: 280, MajorFadeOutMS: 4200, MajorHoldMS: 1400,
		MinorFadeInMS: 180, MinorFadeOutMS: 320, MinorHoldMS: 2800,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
		HardEdges: true, Scanlines: true, CyberRoles: true,
	}
}

func gruvboxTheme() Theme {
	return Theme{
		ID: "gruvbox", Label: "Gruvbox Green",
		FontDisplay: sans, FontBody: plexMono, FontToast: `"Source Serif 4", "Literata", ` + serif,
		BG: RGB{29, 32, 33}, Radius: 8,
		FG: "#ebdbb2", FGMuted: "#a89984", FGDim: "#928374",
		Accent: "#b8bb26", AccentHot: "#98971a",
		Title: "#ebdbb2", Section: "#b8bb26", Border: "#504945",
		OK: "#8ec07c", Warn: "#fe8019", Danger: "#fb4934",
		SigCommon: "#a89984", SigUncommon: "#8ec07c", SigEpic: "#83a598", SigLegendary: "#fabd2f",
		LetterSpacingTitle: "0.02em", UppercaseSection: false, ChipTextShadow: true,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
	}
}

func nordTheme() Theme {
	return Theme{
		ID: "nord", Label: "Nord Frost",
		FontDisplay: sans, FontBody: sans, FontToast: serif, FontQuest: "",
		BG: RGB{46, 52, 64}, Radius: 10,
		FG: "#eceff4", FGMuted: "#d8dee9", FGDim: "#4c566a",
		Accent: "#88c0d0", AccentHot: "#81a1c1",
		Title: "#eceff4", Section: "#8fbcbb", Border: "#4c566a",
		OK: "#a3be8c", Warn: "#ebcb8b", Danger: "#bf616a",
		SigCommon: "#d8dee9", SigUncommon: "#a3be8c", SigEpic: "#b48ead", SigLegendary: "#ebcb8b",
		LetterSpacingTitle: "0.06em", UppercaseSection: true, ChipTextShadow: true,
		MajorFadeInMS: 500, MajorFadeOutMS: 5200,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
	}
}

func roseTheme() Theme {
	lit := `"Literata", "Source Serif 4", ` + serif
	return Theme{
		ID: "rose", Label: "Rose Pine",
		FontDisplay: lit, FontBody: sans, FontToast: lit,
		BG: RGB{25, 23, 36}, Radius: 12,
		FG: "#e0def4", FGMuted: "#908caa", FGDim: "#6e6a86",
		Accent: "#ebbcba", AccentHot: "#c4a7e7",
		Title: "#e0def4", Section: "#c4a7e7", Border: "#26233a",
		OK: "#9ccfd8", Warn: "#f6c177", Danger: "#eb6f92",
		SigCommon: "#908caa", SigUncommon: "#9ccfd8", SigEpic: "#c4a7e7", SigLegendary: "#f6c177",
		LetterSpacingTitle: "0.03em", UppercaseSection: false, ChipTextShadow: true,
		MajorFadeInMS: 560, MajorHoldMS: 1400,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
	}
}

func emberTheme() Theme {
	disp := `"Oxanium", "Rajdhani", "Noto Sans Display", "Noto Sans", sans-serif`
	return Theme{
		ID: "ember", Label: "Ember Forge",
		FontDisplay: disp, FontBody: `"Noto Sans", "DejaVu Sans", sans-serif`, FontToast: serif,
		BG: RGB{22, 12, 10}, Radius: 4,
		FG: "#f4e8dc", FGMuted: "#c4a090", FGDim: "#7a5548",
		Accent: "#ff6b35", AccentHot: "#e85d04",
		Title: "#ffe8d6", Section: "#ff6b35", Border: "#5c2e1f",
		OK: "#80b918", Warn: "#ffba08", Danger: "#d00000",
		SigCommon: "#c4a090", SigUncommon: "#80b918", SigEpic: "#c77dff", SigLegendary: "#ffba08",
		LetterSpacingTitle: "0.1em", UppercaseSection: true, ChipTextShadow: true,
		MajorFadeInMS: 300, MajorFadeOutMS: 4000, MinorFadeInMS: 160, MinorHoldMS: 2600,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
	}
}

func inkTheme() Theme {
	lit := `"Literata", "Source Serif 4", ` + serif
	return Theme{
		ID: "ink", Label: "Ink Mono",
		FontDisplay: lit, FontBody: plexMono, FontToast: lit,
		BG: RGB{12, 12, 12}, Radius: 0,
		FG: "#f5f5f0", FGMuted: "#b0b0a8", FGDim: "#6a6a64",
		Accent: "#f5f5f0", AccentHot: "#d0d0c8",
		Title: "#ffffff", Section: "#f5f5f0", Border: "#3a3a38",
		OK: "#7dce82", Warn: "#e6c35c", Danger: "#e05a5a",
		SigCommon: "#b0b0a8", SigUncommon: "#7dce82", SigEpic: "#9b8ec8", SigLegendary: "#e6c35c",
		LetterSpacingTitle: "0.14em", UppercaseSection: true, ChipTextShadow: true,
		HardEdges:     true,
		MajorFadeInMS: 200, MajorFadeOutMS: 3600, MajorHoldMS: 1000, MinorHoldMS: 2400,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
	}
}

func oceanTheme() Theme {
	return Theme{
		ID: "ocean", Label: "Ocean Drift",
		FontDisplay: sans, FontBody: sans, FontToast: `"Source Serif 4", ` + serif,
		BG: RGB{10, 25, 30}, Radius: 14,
		FG: "#e8f4f2", FGMuted: "#8fb8b0", FGDim: "#4a6e68",
		Accent: "#2ec4b6", AccentHot: "#20a4a0",
		Title: "#e8f4f2", Section: "#2ec4b6", Border: "#1a3a3c",
		OK: "#6bcb77", Warn: "#ffd166", Danger: "#ef476f",
		SigCommon: "#8fb8b0", SigUncommon: "#6bcb77", SigEpic: "#9b5de5", SigLegendary: "#ffd166",
		LetterSpacingTitle: "0.05em", UppercaseSection: false, ChipTextShadow: true,
		MajorFadeInMS: 480, MajorFadeOutMS: 5000,
		ShowSignificance: true, ShowMajorBody: true, ShowStepProgress: true,
	}
}

func quietTheme() Theme {
	lit := `"Literata", "Source Serif 4", ` + serif
	return Theme{
		ID: "quiet", Label: "Тихо",
		FontDisplay: sans, FontBody: sans, FontToast: sans, FontQuest: lit,
		BG: RGB{22, 22, 22}, Radius: 4,
		FG: "#d8d6d0", FGMuted: "#8d8b84", FGDim: "#5c5b56",
		Accent: "#d8d6d0", AccentHot: "#f2f0ea",
		Title: "#f2f0ea", Section: "#f2f0ea", Border: "#3a3936",
		OK: "#9aaa8a", Warn: "#c4a36a", Danger: "#c47a72",
		Step:      "#8a8882",
		SigCommon: "#8d8b84", SigUncommon: "#8d8b84", SigEpic: "#8d8b84", SigLegendary: "#8d8b84",
		LetterSpacingTitle: "0", UppercaseSection: false, ChipTextShadow: false,
		HUDPadY: 8, HUDPadX: 10, HUDMinWidth: 200,
		TitlePT: 12, SectionPT: 11, QuestPT: 11,
		MajorTitlePT: 21, MajorEyebrowPT: 14,
		MajorPadY: 16, MajorPadX: 21, MajorPadBottom: 18,
		MajorFadeInMS: 160, MajorFadeOutMS: 280, MajorHoldMS: 900,
		MinorFadeInMS: 140, MinorFadeOutMS: 220, MinorHoldMS: 1600,
		ShowSignificance: false, ShowMajorBody: false, ShowStepProgress: true,
	}
}
