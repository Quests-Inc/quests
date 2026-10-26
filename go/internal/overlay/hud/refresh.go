//go:build quests_overlay

package hud

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/overlay/model"
	"github.com/quests-inc/quests/go/internal/overlay/notices"
	"github.com/quests-inc/quests/go/internal/overlay/stylepack"
)

func (h *Host) refresh(refetch bool) {
	gen := h.refreshGen.Add(1)
	if !refetch {
		h.apply(h.cache, h.categories, "")
		return
	}
	go func() {
		quests, qErr := h.client.FetchQuests()
		cats, cErr := h.client.FetchCategories()
		glib.IdleAdd(func() bool {
			if gen != h.refreshGen.Load() {
				return false
			}
			// Match Python: keep last cache on transient errors; only show err when empty.
			if qErr == nil {
				h.cache = quests
				h.lastErr = ""
			} else if len(h.cache) == 0 {
				h.lastErr = qErr.Error()
			} else {
				h.lastErr = ""
			}
			if cErr == nil {
				h.categories = cats
			}
			h.apply(h.cache, h.categories, h.lastErr)
			h.scheduleInputSync()
			return false
		})
	}()
}

func (h *Host) repaint() {
	h.apply(h.cache, h.categories, h.lastErr)
}

func (h *Host) apply(items []api.Quest, cats []api.Category, errMsg string) {
	slug, label := model.ResolveCategory(cats, h.catSlug)
	if slug != h.catSlug && slug != "" {
		h.catSlug = slug
		h.cfg.HUDCategory = slug
		_ = config.Save(h.cfg)
	}
	h.catLabel = label
	fav, urg, cat := model.SplitQuests(items, slug, time.Now())
	pm := panelModel{
		Favorites: fav, Urgent: urg, Category: cat,
		CatLabel: label, ErrMsg: errMsg,
		Collapsed: h.collapsed, Interactive: h.interactive,
		SettingsOpen: h.settingsOpen, SettingsFocus: h.settingsFocus,
		Cfg: h.cfg, Categories: cats,
	}
	// The 1s timer tick must not rebuild the widget tree: each rebuild leaked
	// native memory. Rebuild only when the structure changes; otherwise just
	// update the countdown labels in place.
	sig := panelSignature(pm)
	if sig == h.panelSig && !dbgOn("forcerebuild") && h.updateTimers(pm) {
		return
	}
	if dbgOn("logrebuild") {
		i := 0
		for i < len(sig) && i < len(h.panelSig) && sig[i] == h.panelSig[i] {
			i++
		}
		lo, hi := max(i-40, 0), min(i+60, len(sig))
		log.Printf("hud: rebuild panel, first diff at %d: ...%s", i, sig[lo:hi])
	}
	h.panelSig = sig
	child, timers := buildPanel(pm)
	h.timers = timers
	h.win.SetChild(child)
	h.syncModeChrome()
	// SetChild / layout often resets the Wayland input region.
	h.scheduleInputSync()
}

func (h *Host) pollEvents() {
	go func() {
		rev, events, err := h.client.FetchEvents(h.revision)
		if err != nil {
			return
		}
		parsed := make([]notices.Event, 0, len(events))
		for _, raw := range events {
			parsed = append(parsed, parseEvent(raw))
		}
		glib.IdleAdd(func() bool {
			h.revision = rev
			if len(events) > 0 {
				h.refresh(true)
				for _, ev := range parsed {
					h.notices.Enqueue(ev)
				}
			}
			go h.client.PostHeartbeat("go-hud")
			return false
		})
	}()
}

func (h *Host) tickTimers() {
	if len(h.cache) == 0 {
		return
	}
	// Don't rebuild settings mid-edit (would reset sliders).
	if h.settingsOpen {
		return
	}
	h.apply(h.cache, h.categories, h.lastErr)
	h.scheduleInputSync()
}

func parseEvent(raw map[string]any) notices.Event {
	ev := notices.Event{Toast: true}
	if v, ok := raw["kind"].(string); ok {
		ev.Kind = v
	}
	if v, ok := raw["title"].(string); ok {
		ev.Title = v
	}
	if v, ok := raw["description"].(string); ok {
		ev.Description = v
	}
	if v, ok := raw["detail"].(string); ok {
		ev.Detail = v
	}
	if v, ok := raw["significance"].(string); ok {
		ev.Significance = v
	}
	if v, ok := raw["automated"].(bool); ok {
		ev.Automated = v
	}
	if v, ok := raw["toast"].(bool); ok {
		ev.Toast = v
	}
	return ev
}

// panelSignature is a value-based fingerprint of the layout. It must not use
// %v: QuestView holds pointers, whose addresses differ on every API refetch
// and forced a full rebuild (and native memory leak) every refresh.
func panelSignature(pm panelModel) string {
	b, err := json.Marshal(structOnly(pm))
	if err != nil {
		return fmt.Sprintf("err:%v:%d", err, time.Now().UnixNano())
	}
	return stylepack.ActiveID() + string(b)
}

// structOnly blanks countdown text/tone (keeping their presence) so the
// signature changes only when the layout does.
func structOnly(pm panelModel) panelModel {
	strip := func(in []model.QuestView) []model.QuestView {
		out := make([]model.QuestView, len(in))
		for i, q := range in {
			if q.TimerLabel != "" {
				q.TimerLabel = "*"
			}
			q.TimerTone = ""
			out[i] = q
		}
		return out
	}
	pm.Favorites, pm.Urgent, pm.Category = strip(pm.Favorites), strip(pm.Urgent), strip(pm.Category)
	return pm
}

// updateTimers rewrites countdown labels in place; false if the label set
// does not match the built widgets (caller rebuilds).
func (h *Host) updateTimers(pm panelModel) bool {
	var views []model.QuestView
	views = append(views, pm.Favorites...)
	views = append(views, pm.Urgent...)
	views = append(views, pm.Category...)
	i := 0
	for _, q := range views {
		if q.TimerLabel == "" {
			continue
		}
		if i >= len(h.timers) {
			return false
		}
		t := h.timers[i]
		i++
		if t.label.Label() != q.TimerLabel {
			t.label.SetLabel(q.TimerLabel)
		}
		tone := q.TimerTone
		if tone == "" {
			tone = "green"
		}
		if tone != t.tone {
			t.label.RemoveCSSClass("hud-timer--" + t.tone)
			t.label.AddCSSClass("hud-timer--" + tone)
			t.tone = tone
		}
	}
	return i == len(h.timers)
}
