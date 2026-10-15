//go:build quests_overlay

package stylepack

import (
	"fmt"
	"strings"
)

// BuildCSS renders GTK CSS for the Go HUD + notices class names.
func BuildCSS(t Theme, look Look) string {
	t = t.withDefaults()
	mode := strings.ToLower(strings.TrimSpace(look.Mode))
	if mode != "full" {
		mode = "chips"
	}
	panelA := look.PanelA
	if panelA <= 0 || panelA > 1 {
		panelA = 0.6
	}
	textA := look.TextA
	if textA <= 0 || textA > 1 {
		textA = 0.92
	}

	rad := t.Radius
	if t.HardEdges {
		rad = 0
	}
	radSm := rad
	if radSm > 2 {
		radSm = rad - 2
	}

	plate := rgbaRGB(t.BG, 0.72)
	borderInteractive := hexAlpha(t.Border, 0.75)
	fg := hexAlpha(t.FG, textA)
	dim := hexAlpha(t.FGMuted, textA*0.85)
	stepCol := hexAlpha(t.StepColor(), textA*0.9)
	titleCol := hexAlpha(t.Title, textA)
	sectionCol := hexAlpha(t.Section, textA)

	sectionTransform := "none"
	sectionTracking := "0.04em"
	if t.UppercaseSection {
		sectionTransform = "uppercase"
		sectionTracking = "0.12em"
	}

	shadow := "text-shadow: none;"
	if t.ChipTextShadow {
		shadow = `text-shadow: 0 0 3px rgba(0,0,0,0.9), 0 1px 2px rgba(0,0,0,0.85);`
	}

	scanBG := ""
	if t.Scanlines {
		scanBG = fmt.Sprintf(
			`background-image: repeating-linear-gradient(0deg, transparent, transparent 1px, rgba(0,0,0,0.07) 1px, rgba(0,0,0,0.07) 2px);`)
	}

	// Cyber role overrides
	headerCol := sectionCol
	questCol := titleCol
	if t.CyberRoles {
		headerCol = hexAlpha("#00f0ff", textA)
		questCol = hexAlpha("#e03131", textA)
		stepCol = hexAlpha("#fcee0a", textA)
	}

	// Passthrough look: chips = per-row bars on .hud-row only; full = window plate.
	passBlock := ""
	if mode == "full" {
		borderA := panelA + 0.12
		if borderA > 1 {
			borderA = 1
		}
		passBlock = fmt.Sprintf(`
window.quests-hud.hud-window--passthrough {
  background-color: %s;
  border: 1px solid %s;
  border-radius: %dpx;
}
.hud:not(.hud--interactive) {
  padding: %dpx %dpx;
  min-width: %dpx;
}
.hud:not(.hud--interactive) .hud-chip {
  background-color: transparent;
  background-image: none;
  border-radius: 0;
  padding: 0;
}
.hud:not(.hud--interactive) .hud-row {
  background-color: transparent;
  padding: 0;
}
`, rgbaRGB(t.BG, panelA), rgbaRGB(t.BG, borderA), rad, t.HUDPadY, t.HUDPadX, t.HUDMinWidth)
	} else {
		passBlock = fmt.Sprintf(`
window.quests-hud.hud-window--passthrough {
  background-color: transparent;
  border: none;
  border-radius: 0;
}
.hud:not(.hud--interactive) {
  background-color: transparent;
  padding: 0;
  min-width: 0;
}
.hud:not(.hud--interactive) .hud-chip {
  background-color: transparent;
  background-image: none;
  border-radius: 0;
  padding: 0;
}
.hud:not(.hud--interactive) .hud-row {
  background-color: %s;
  background-image: none;
  border-radius: %dpx;
  padding: 2px 8px;
  min-width: 0;
  min-height: 0;
}
.hud:not(.hud--interactive) .hud-section,
.hud:not(.hud--interactive) .hud-card,
.hud:not(.hud--interactive) .hud-header {
  background-color: transparent;
  background-image: none;
  padding: 0;
  margin: 0;
  border: none;
}
`, rgbaRGB(t.BG, panelA), radSm)
	}

	leftMajor := 2
	if t.HardEdges {
		leftMajor = 3
	}
	if t.ID == "ink" {
		leftMajor = 4
	}

	return fmt.Sprintf(`
window.quests-hud {
  background: transparent;
  border: none;
}
window.quests-hud.hud-window--interactive {
  background: %s;
  border: 2px solid %s;
  border-radius: %dpx;
  %s
}
window.quests-hud > box {
  background: transparent;
}
.hud-root {
  background: transparent;
  font-family: %s;
  color: %s;
}
.hud-card, .hud-section {
  background: transparent;
  color: %s;
}
.hud-row {
  background: transparent;
}
.hud--interactive .hud-row {
  background: transparent;
  padding: 0;
}
.hud-lane {
  color: %s;
  font-family: %s;
  font-size: %dpt;
  font-weight: 700;
  letter-spacing: %s;
  text-transform: %s;
  margin-top: 4px;
}
.hud-header {
  color: %s;
  font-family: %s;
  font-weight: 700;
  font-size: %dpt;
  letter-spacing: %s;
}
.hud-title {
  color: %s;
  font-family: %s;
  font-weight: 600;
  font-size: %dpt;
  %s
}
.hud-step {
  color: %s;
  font-size: 0.9em;
  %s
}
.hud-timer--green { color: %s; }
.hud-timer--orange { color: %s; }
.hud-timer--red, .hud-timer--overdue { color: %s; }
%s
window.quests-major, window.quests-minor {
  background: transparent;
}
.notice-root { background: transparent; }
.major {
  background: %s;
  border: 1px solid %s;
  border-left-width: %dpx;
  border-radius: %dpx;
  padding: %dpx %dpx %dpx;
  color: %s;
  font-family: %s;
  %s
}
.major__eyebrow {
  font-size: %dpt;
  color: %s;
  letter-spacing: 0.08em;
  text-transform: %s;
}
.major__title {
  font-size: %dpt;
  font-weight: 700;
  color: %s;
}
.major__body {
  font-size: 0.95em;
  opacity: 0.85;
  color: %s;
}
.major--sig-common .major__eyebrow { color: %s; }
.major--sig-uncommon .major__eyebrow { color: %s; }
.major--sig-epic .major__eyebrow { color: %s; }
.major--sig-legendary .major__eyebrow { color: %s; }
.minor {
  background: %s;
  border: 1px solid %s;
  border-radius: %dpx;
  padding: 10px 14px;
  color: %s;
  font-family: %s;
  min-width: 240px;
}
.minor__title { font-weight: 600; color: %s; }
.minor__change, .minor__detail {
  font-size: 0.85em;
  opacity: 0.8;
  color: %s;
}
button.hud-icon-btn {
  color: %s;
  font-family: %s;
  font-size: 13pt;
  font-weight: 600;
  min-width: 1.6em;
  min-height: 1.6em;
  padding: 2px 4px;
  margin: 0;
  background: transparent;
  border: none;
  border-radius: %dpx;
  opacity: 0.72;
}
button.hud-icon-btn:hover {
  color: %s;
  background-color: %s;
  opacity: 1;
}
.hud-settings-panel {
  min-width: 280px;
  padding: 2px 0 4px;
}
.hud-settings-sep {
  min-height: 1px;
  margin: 4px 0 2px;
  background-color: %s;
}
.hud-settings-label {
  color: %s;
  font-family: %s;
  font-size: 9pt;
  font-weight: 600;
  letter-spacing: 0.04em;
}
.hud-settings-label--focus {
  color: %s;
  font-weight: 700;
}
.hud-settings-value {
  color: %s;
  font-family: %s;
  font-size: 10pt;
  opacity: 0.9;
}
.hud-settings-value--focus {
  color: %s;
  font-weight: 700;
  opacity: 1;
}
.hud-settings-row--focus {
  background-color: %s;
  border-radius: %dpx;
  padding: 4px 6px;
}
.hud-settings-section {
  color: %s;
  font-family: %s;
  font-size: 10pt;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: %s;
  margin-top: 2px;
  opacity: 0.95;
}
.hud-settings-hint {
  font-size: 8pt;
  opacity: 0.75;
  color: %s;
}
.hud-opt-slider {
  border: 1px solid %s;
  border-radius: %dpx;
  overflow: hidden;
  background-color: %s;
}
button.hud-opt {
  color: %s;
  font-family: %s;
  font-size: 9pt;
  font-weight: 500;
  min-width: 0;
  min-height: 0;
  padding: 6px 10px;
  margin: 0;
  border: none;
  border-radius: 0;
  border-right: 1px solid %s;
  background: transparent;
}
button.hud-opt--last { border-right: none; }
button.hud-opt:hover { background-color: %s; }
button.hud-opt--on {
  background-color: %s;
  font-weight: 700;
}
.hud-settings-scale {
  color: %s;
  font-family: %s;
  font-size: 9pt;
  margin-top: 2px;
}
.hud-settings-scale trough {
  min-height: 6px;
  border-radius: 3px;
  background-color: %s;
}
.hud-settings-scale highlight {
  background-color: %s;
  border-radius: 3px;
}
.hud-settings-scale slider {
  min-width: 12px;
  min-height: 12px;
  border-radius: 6px;
  background-color: %s;
  border: none;
}
`,
		plate, borderInteractive, rad, scanBG,
		t.FontBody, fg,
		fg,
		headerCol, t.FontDisplay, t.SectionPT, sectionTracking, sectionTransform,
		headerCol, t.FontDisplay, t.TitlePT, t.LetterSpacingTitle,
		questCol, t.QuestFont(), t.QuestPT, shadow,
		stepCol, shadow,
		hexAlpha(t.OK, textA), hexAlpha(t.Warn, textA), hexAlpha(t.Danger, textA),
		passBlock,
		plate, hexAlpha(t.Border, 0.55), leftMajor, rad,
		t.MajorPadY, t.MajorPadX, t.MajorPadBottom,
		fg, t.FontToast, scanBG,
		t.MajorEyebrowPT, hexAlpha(t.Accent, textA), sectionTransform,
		t.MajorTitlePT, titleCol,
		dim,
		hexAlpha(t.SigCommon, 1), hexAlpha(t.SigUncommon, 1), hexAlpha(t.SigEpic, 1), hexAlpha(t.SigLegendary, 1),
		rgbaRGB(t.BG, 0.78), hexAlpha(t.Border, 0.45), radSm,
		fg, t.FontToast,
		titleCol, dim,
		// icon btn
		hexAlpha(t.Accent, 1), t.FontBody, max(2, rad/2),
		fg, hexAlpha(t.Accent, 0.18),
		// settings
		hexAlpha(t.Accent, 0.35),
		hexAlpha(t.Accent, 1), t.FontBody,
		hexAlpha(t.AccentHot, 1),
		fg, t.FontBody,
		titleCol,
		hexAlpha(t.Accent, 0.16), radSm,
		fg, t.FontDisplay, sectionTransform,
		dim,
		hexAlpha(t.Border, 0.55), radSm, rgbaRGB(t.BG, 0.45),
		fg, t.FontBody, hexAlpha(t.Border, 0.35),
		hexAlpha(t.Accent, 0.14), hexAlpha(t.Accent, 0.28),
		fg, t.FontBody,
		hexAlpha(t.Accent, 0.18), hexAlpha(t.Accent, 0.55), hexAlpha(t.Accent, 1),
	)
}
