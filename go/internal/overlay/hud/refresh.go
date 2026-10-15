//go:build quests_overlay

package hud

import (
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/overlay/model"
	"github.com/quests-inc/quests/go/internal/overlay/notices"
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
	h.win.SetChild(buildPanel(panelModel{
		Favorites: fav, Urgent: urg, Category: cat,
		CatLabel: label, ErrMsg: errMsg,
		Collapsed: h.collapsed, Interactive: h.interactive,
		SettingsOpen: h.settingsOpen, SettingsFocus: h.settingsFocus,
		Cfg: h.cfg, Categories: cats,
	}))
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
