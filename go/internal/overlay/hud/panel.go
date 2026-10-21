//go:build quests_overlay

package hud

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/overlay/model"
	"github.com/quests-inc/quests/go/internal/overlay/stylepack"
)

type panelModel struct {
	Favorites, Urgent, Category []model.QuestView
	CatLabel, ErrMsg            string
	Collapsed, Interactive      bool
	SettingsOpen                bool
	SettingsFocus               int
	Cfg                         config.Config
	Categories                  []api.Category
}

// timerRef is a countdown label that can be updated in place, so the 1s tick
// does not rebuild (and leak native memory from) the whole widget tree.
type timerRef struct {
	label *gtk.Label
	tone  string
}

// buildPanel renders quests list or keyboard-driven settings (no mouse targets).
func buildPanel(m panelModel) (gtk.Widgetter, []*timerRef) {
	var timers []*timerRef
	pack := stylepack.Active()
	showSettings := m.Interactive && m.SettingsOpen && !m.Collapsed

	root := gtk.NewBox(gtk.OrientationVertical, 8)
	root.AddCSSClass("hud-root")
	root.AddCSSClass("hud")
	if m.Collapsed {
		root.AddCSSClass("hud--collapsed")
	}
	if m.Interactive {
		root.AddCSSClass("hud--interactive")
	}
	if showSettings {
		root.AddCSSClass("hud--settings")
	}
	root.SetMarginStart(6)
	root.SetMarginEnd(6)
	root.SetMarginTop(4)
	root.SetMarginBottom(4)
	root.SetHAlign(gtk.AlignEnd)
	root.SetHExpand(false)

	titleText := "Задачи"
	if showSettings {
		titleText = "Настройки"
	}
	header := gtk.NewLabel(titleText)
	header.AddCSSClass("hud-header")
	header.AddCSSClass("title")
	header.SetXAlign(1)
	header.SetHAlign(gtk.AlignEnd)
	root.Append(header)

	if m.Interactive && !showSettings {
		keys := gtk.NewLabel("s · настройки   Esc · выход   Bksp · свернуть")
		keys.AddCSSClass("hint")
		keys.AddCSSClass("hud-settings-hint")
		keys.SetHAlign(gtk.AlignEnd)
		keys.SetXAlign(1)
		root.Append(keys)
	}

	switch {
	case m.Collapsed:
		// header only
	case showSettings:
		appendSettingsPanel(root, m.Cfg, m.Categories, m.SettingsFocus)
	case len(m.Favorites) == 0 && len(m.Urgent) == 0 && len(m.Category) == 0:
		// Keep last cache on transient API blips; only surface the error when empty.
		msg := "нет активных"
		if m.ErrMsg != "" {
			msg = m.ErrMsg
		}
		empty := gtk.NewLabel(msg)
		empty.SetXAlign(1)
		empty.SetHAlign(gtk.AlignEnd)
		empty.SetWrap(true)
		empty.AddCSSClass("hud-step")
		root.Append(hudRow(empty))
	default:
		appendLane(root, "Избранное", m.Favorites, pack, m.Interactive, &timers)
		appendLane(root, "Срочные", m.Urgent, pack, m.Interactive, &timers)
		title := "Категория"
		if m.CatLabel != "" {
			title = m.CatLabel
		}
		appendLane(root, title, m.Category, pack, m.Interactive, &timers)
	}

	return root, timers
}

func appendLane(root *gtk.Box, title string, items []model.QuestView, pack stylepack.Theme, interactive bool, timers *[]*timerRef) {
	if len(items) == 0 {
		return
	}

	lane := gtk.NewLabel(title)
	lane.SetXAlign(1)
	lane.SetHAlign(gtk.AlignEnd)
	lane.AddCSSClass("hud-lane")
	lane.AddCSSClass("section-title")
	lane.AddCSSClass("hud-chip")
	root.Append(hudRow(lane))

	for _, q := range items {
		section := gtk.NewBox(gtk.OrientationVertical, 2)
		section.AddCSSClass("hud-section")
		section.AddCSSClass("hud-card")
		section.SetHAlign(gtk.AlignEnd)
		section.SetHExpand(false)

		var titleKids []gtk.Widgetter
		name := gtk.NewLabel(q.Title)
		name.AddCSSClass("hud-title")
		name.AddCSSClass("quest-title")
		name.AddCSSClass("hud-chip")
		name.SetXAlign(1)
		name.SetHAlign(gtk.AlignEnd)
		name.SetWrap(true)
		titleKids = append(titleKids, name)

		if q.TimerLabel != "" {
			timer := gtk.NewLabel(q.TimerLabel)
			timer.SetXAlign(1)
			timer.SetHAlign(gtk.AlignEnd)
			timer.AddCSSClass("hud-chip")
			tone := q.TimerTone
			if tone == "" {
				tone = "green"
			}
			timer.AddCSSClass("hud-timer--" + tone)
			*timers = append(*timers, &timerRef{label: timer, tone: tone})
			titleKids = append(titleKids, timer)
		}
		section.Append(hudRow(titleKids...))

		for _, s := range q.Steps {
			line := s.Title
			if pack.ShowStepProgress && s.Progress != "" {
				line = fmt.Sprintf("%s  %s", s.Title, s.Progress)
			}
			step := gtk.NewLabel(line)
			step.SetXAlign(1)
			step.SetHAlign(gtk.AlignEnd)
			step.SetWrap(true)
			step.AddCSSClass("hud-step")
			step.AddCSSClass("quest-title")
			step.AddCSSClass("hud-chip")
			section.Append(hudRow(step))
		}

		_ = interactive
		root.Append(section)
	}
}

// hudRow right-aligns a painted plate so chip backgrounds hug content
// (vertical Gtk.Box otherwise stretches short lines into ghost bars).
func hudRow(children ...gtk.Widgetter) *gtk.Box {
	outer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	outer.SetHAlign(gtk.AlignFill)
	outer.SetHExpand(true)

	pad := gtk.NewBox(gtk.OrientationHorizontal, 0)
	pad.SetHExpand(true)
	pad.SetHAlign(gtk.AlignFill)

	plate := gtk.NewBox(gtk.OrientationHorizontal, 8)
	plate.AddCSSClass("hud-row")
	plate.SetHAlign(gtk.AlignEnd)
	plate.SetHExpand(false)
	for _, c := range children {
		plate.Append(c)
	}

	outer.Append(pad)
	outer.Append(plate)
	return outer
}
