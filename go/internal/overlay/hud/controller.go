//go:build quests_overlay

package hud

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/overlay/ipc"
	"github.com/quests-inc/quests/go/internal/overlay/model"
	"github.com/quests-inc/quests/go/internal/overlay/notices"
	"github.com/quests-inc/quests/go/internal/overlay/shell"
	"github.com/quests-inc/quests/go/internal/overlay/stylepack"
)

// dbg holds comma-separated QUESTS_DBG switches used to bisect the native
// memory leak (quest=478): nopoll, norefresh, notick.
var dbg = os.Getenv("QUESTS_DBG")

func dbgOn(flag string) bool { return strings.Contains(dbg, flag) }

const (
	eventPollMS = 2500
	dataSyncMS  = 15000
	timerTickMS = 1000
)

// Host owns the HUD window and refresh loop.
type Host struct {
	app           *gtk.Application
	win           *gtk.ApplicationWindow
	client        *api.Client
	cfg           config.Config
	notices       *notices.Router
	interactive   bool
	collapsed     bool
	settingsOpen  bool
	settingsFocus int

	revision   int64
	catSlug    string
	catLabel   string
	cache      []api.Quest
	categories []api.Category
	lastErr    string
	refreshGen atomic.Uint64
	panelSig   string // structure of the last rendered panel (timers excluded)
	timers     []*timerRef
	inputGen   atomic.Uint64
	inputHooks bool
	ipc        *ipc.Server
}

// Activate builds the HUD and starts pollers + IPC.
func Activate(app *gtk.Application) {
	cfg := config.Load()
	h := &Host{
		app:    app,
		client: api.New(cfg),
		cfg:    cfg,
	}
	h.catSlug = cfg.HUDCategory

	if display := gdk.DisplayGetDefault(); display != nil {
		stylepack.Apply(display, cfg.StylePack, stylepack.Look{
			Mode:   cfg.PassthroughBG,
			PanelA: cfg.PassthroughAlpha,
			TextA:  cfg.HUDTextAlpha,
		})
	}

	win := gtk.NewApplicationWindow(app)
	win.SetTitle("Quests Overlay")
	win.SetDecorated(false)
	win.SetDefaultSize(340, -1)
	win.AddCSSClass("quests-hud")
	shell.PrepareHUD(&win.Window, cfg.MarginTop, cfg.MarginRight)
	h.win = win
	h.syncModeChrome()
	h.notices = notices.NewRouter(app, cfg.ToastsMajor, cfg.ToastsMinorMode)
	h.attachKeys()
	h.applyMonitor()

	loading, _ := buildPanel(panelModel{ErrMsg: "загрузка…"})
	win.SetChild(loading)
	win.ConnectRealize(func() {
		_ = shell.ApplyInputMode(&win.Window, false)
		h.attachInputHooks()
		h.scheduleInputSync()
	})
	win.ConnectMap(func() {
		_ = shell.ApplyInputMode(&win.Window, h.interactive)
		h.scheduleInputSync()
	})
	// Realize before Present so the first frame already has an empty input region
	// (default region = full surface → cursor/hot-corners break).
	win.Realize()
	_ = shell.ApplyInputMode(&win.Window, false)
	win.Present()
	shell.DisableKeyboardFocus(&win.Window)
	h.attachInputHooks()
	h.scheduleInputSync()

	dispatch := func(fn func()) {
		glib.IdleAdd(func() bool {
			fn()
			return false
		})
	}
	srv, err := ipc.Start(h.handleIPC, dispatch)
	if err != nil {
		h.lastErr = "ipc: " + err.Error()
	} else {
		h.ipc = srv
		app.ConnectShutdown(func() {
			h.ipc.Stop()
		})
	}

	h.refresh(true)
	if !dbgOn("nopoll") {
		glib.TimeoutAdd(uint(eventPollMS), func() bool {
			h.pollEvents()
			return true
		})
	}
	if !dbgOn("norefresh") {
		glib.TimeoutAdd(uint(dataSyncMS), func() bool {
			h.refresh(true)
			return true
		})
	}
	if !dbgOn("notick") {
		glib.TimeoutAdd(uint(timerTickMS), func() bool {
			h.tickTimers()
			return true
		})
	}
	go h.client.PostHeartbeat("go-hud")
}

func (h *Host) handleIPC(cmd string) string {
	switch cmd {
	case "status":
		fav, urg, cat := model.SplitQuests(h.cache, h.catSlug, time.Now())
		token := "no"
		if h.client.Token != "" {
			token = "yes"
		}
		mode := "passthrough"
		if h.interactive {
			mode = "interactive"
		}
		fold := "expanded"
		if h.collapsed {
			fold = "collapsed"
		}
		return fmt.Sprintf(
			"ok go-hud mode=%s fold=%s style=%s mon=%s api=%s token=%s revision=%d quests=%d fav=%d urg=%d cat=%d err=%q",
			mode, fold, stylepack.ActiveID(), h.hudConnector(), h.client.Base, token, h.revision, len(h.cache),
			len(fav), len(urg), len(cat), h.lastErr,
		)
	case "toggle", "toggle-input":
		return h.setInteractive(!h.interactive)
	case "interactive on":
		return h.setInteractive(true)
	case "interactive off":
		return h.setInteractive(false)
	case "monitor", "next-monitor", "cycle-monitor":
		return h.cycleMonitor()
	case "refresh":
		h.refresh(true)
		return "refresh"
	case "style", "style list", "style status":
		return h.styleStatus()
	default:
		if strings.HasPrefix(cmd, "style ") {
			name := strings.TrimSpace(strings.TrimPrefix(cmd, "style "))
			if name == "" || name == "list" || name == "status" {
				return h.styleStatus()
			}
			return h.setStylePack(name)
		}
		if cmd == "" {
			return "error: empty"
		}
		return "error: unknown " + cmd
	}
}

func (h *Host) styleStatus() string {
	ids := make([]string, 0, len(stylepack.List()))
	for _, p := range stylepack.List() {
		ids = append(ids, p.ID)
	}
	return fmt.Sprintf("style: %s [%s]", stylepack.ActiveID(), strings.Join(ids, ", "))
}

func (h *Host) setStylePack(name string) string {
	h.cfg.StylePack = name
	theme := stylepack.Resolve(name)
	h.cfg.StylePack = theme.ID
	_ = config.Save(h.cfg)
	h.reloadStyle()
	h.repaint()
	return fmt.Sprintf("style: %s", theme.ID)
}

func (h *Host) setInteractive(on bool) string {
	if on {
		// Seat focus must be on the HUD's output or layer-shell keys are dropped.
		shell.FocusNiriOutput(h.hudConnector())
		h.collapsed = false
	} else {
		h.settingsOpen = false
	}
	h.interactive = on
	h.syncModeChrome()
	// Rebuild first — SetChild resets the Wayland input region to the default
	// (full surface). Stamp the desired region only after that.
	h.repaint()
	if on {
		h.win.Present()
		_ = shell.ApplyInputMode(&h.win.Window, true)
		glib.IdleAdd(func() bool {
			h.win.GrabFocus()
			return false
		})
	} else {
		shell.DisableKeyboardFocus(&h.win.Window)
		_ = shell.ApplyInputMode(&h.win.Window, false)
	}
	h.scheduleInputSync()
	if on {
		return "interactive"
	}
	return "passthrough"
}

func (h *Host) toggleSettings() {
	if !h.interactive || h.collapsed {
		return
	}
	h.settingsOpen = !h.settingsOpen
	if h.settingsOpen {
		h.settingsFocus = 0
	}
	h.repaint()
}

func (h *Host) syncModeChrome() {
	if h.interactive {
		h.win.AddCSSClass("hud-window--interactive")
		h.win.RemoveCSSClass("hud-window--passthrough")
	} else {
		h.win.RemoveCSSClass("hud-window--interactive")
		h.win.AddCSSClass("hud-window--passthrough")
	}
	if h.collapsed {
		h.win.AddCSSClass("hud-window--collapsed")
	} else {
		h.win.RemoveCSSClass("hud-window--collapsed")
	}
}

func (h *Host) setCollapsed(on bool) string {
	h.collapsed = on
	if on {
		h.interactive = false
		h.settingsOpen = false
	}
	h.syncModeChrome()
	h.repaint()
	if on {
		shell.DisableKeyboardFocus(&h.win.Window)
		_ = shell.ApplyInputMode(&h.win.Window, false)
	}
	h.scheduleInputSync()
	if on {
		return "collapsed"
	}
	return "expanded"
}

func (h *Host) selectMonitor(index int) {
	mons := shell.ListMonitors(gdk.DisplayGetDefault())
	if len(mons) == 0 {
		return
	}
	h.cfg.MonitorConnector = ""
	h.cfg.MonitorIndex = index % len(mons)
	_ = h.applyMonitor()
	shell.SetMargins(&h.win.Window, h.cfg.MarginTop, h.cfg.MarginRight)
	_ = config.Save(h.cfg)
	h.repaint()
	if h.interactive {
		shell.FocusNiriOutput(h.hudConnector())
	}
}

func (h *Host) selectCategory(slug string) {
	h.catSlug = strings.TrimSpace(slug)
	h.cfg.HUDCategory = h.catSlug
	_ = config.Save(h.cfg)
	h.repaint()
}

func (h *Host) setHUDLook(mode string, alpha, textAlpha float64) {
	prevMode := h.cfg.PassthroughBG
	h.cfg.PassthroughBG = mode
	if h.cfg.PassthroughBG != "full" {
		h.cfg.PassthroughBG = "chips"
	}
	if alpha >= 0 && alpha <= 1 {
		h.cfg.PassthroughAlpha = alpha
	}
	if textAlpha >= 0 && textAlpha <= 1 {
		h.cfg.HUDTextAlpha = textAlpha
	}
	_ = config.Save(h.cfg)
	h.reloadStyle()
	// Rebuild only when chips highlight must change (mode switch).
	if h.cfg.PassthroughBG != prevMode {
		h.repaint()
	}
}

func (h *Host) setMajorToasts(on bool) {
	h.cfg.ToastsMajor = on
	_ = config.Save(h.cfg)
	if h.notices != nil {
		h.notices.SetMajorEnabled(on)
	}
	h.repaint()
}

func (h *Host) setMinorToasts(mode, bgMode string, bgA, textA float64, width, height int) {
	prevMode, prevBG := h.cfg.ToastsMinorMode, h.cfg.MinorBGMode
	h.cfg.ToastsMinorMode = mode
	if h.cfg.ToastsMinorMode != "off" && h.cfg.ToastsMinorMode != "toast" && h.cfg.ToastsMinorMode != "log" {
		h.cfg.ToastsMinorMode = "toast"
	}
	h.cfg.MinorBGMode = bgMode
	if h.cfg.MinorBGMode != "full" {
		h.cfg.MinorBGMode = "chips"
	}
	if bgA >= 0 && bgA <= 1 {
		h.cfg.MinorBGAlpha = bgA
	}
	if textA >= 0 && textA <= 1 {
		h.cfg.MinorTextAlpha = textA
	}
	if width > 0 {
		h.cfg.MinorLogWidth = width
	}
	if height > 0 {
		h.cfg.MinorLogHeight = height
	}
	_ = config.Save(h.cfg)
	if h.notices != nil {
		h.notices.SetMinorMode(h.cfg.ToastsMinorMode)
	}
	if h.cfg.ToastsMinorMode != prevMode || h.cfg.MinorBGMode != prevBG {
		h.repaint()
	}
}

func (h *Host) reloadStyle() {
	stylepack.Apply(gdk.DisplayGetDefault(), h.cfg.StylePack, stylepack.Look{
		Mode:   h.cfg.PassthroughBG,
		PanelA: h.cfg.PassthroughAlpha,
		TextA:  h.cfg.HUDTextAlpha,
	})
}

func (h *Host) cycleMonitor() string {
	mons := shell.ListMonitors(gdk.DisplayGetDefault())
	if len(mons) == 0 {
		return "no monitors"
	}
	h.cfg.MonitorConnector = ""
	h.cfg.MonitorIndex = (h.cfg.MonitorIndex + 1) % len(mons)
	result := h.applyMonitor()
	shell.SetMargins(&h.win.Window, h.cfg.MarginTop, h.cfg.MarginRight)
	_ = config.Save(h.cfg)
	h.repaint()
	if h.interactive {
		shell.FocusNiriOutput(h.hudConnector())
	}
	return result
}

func (h *Host) cycleCategory(delta int) {
	if len(h.categories) == 0 {
		go func() {
			cats, err := h.client.FetchCategories()
			if err != nil {
				return
			}
			glib.IdleAdd(func() bool {
				h.categories = cats
				h.cycleCategory(delta)
				return false
			})
		}()
		return
	}
	next := model.CycleCategory(h.categories, h.catSlug, delta)
	if next == "" {
		return
	}
	h.catSlug = next
	h.cfg.HUDCategory = next
	_ = config.Save(h.cfg)
	h.repaint()
}

func (h *Host) applyMonitor() string {
	mons := shell.ListMonitors(gdk.DisplayGetDefault())
	if len(mons) == 0 {
		shell.ApplyMonitor(&h.win.Window, nil)
		if h.notices != nil {
			h.notices.SetMonitor(nil)
		}
		return "monitor: default"
	}
	idx := shell.ResolveMonitorIndex(mons, h.cfg.MonitorConnector, h.cfg.MonitorIndex)
	h.cfg.MonitorIndex = idx
	mon := mons[idx]
	shell.ApplyMonitor(&h.win.Window, mon)
	if h.notices != nil {
		h.notices.SetMonitor(mon)
	}
	conn := shell.MonitorConnector(mon)
	if conn != "" {
		h.cfg.MonitorConnector = conn
	}
	return fmt.Sprintf("monitor: %s (%d/%d)", conn, idx+1, len(mons))
}

func (h *Host) hudConnector() string {
	if c := h.cfg.MonitorConnector; c != "" {
		return c
	}
	mons := shell.ListMonitors(gdk.DisplayGetDefault())
	if len(mons) == 0 {
		return ""
	}
	idx := shell.ResolveMonitorIndex(mons, "", h.cfg.MonitorIndex)
	return shell.MonitorConnector(mons[idx])
}

func (h *Host) persistMargins() {
	_ = config.Save(h.cfg)
}

func (h *Host) attachInputHooks() {
	if h.inputHooks {
		return
	}
	surface := shell.SurfaceOf(&h.win.Window)
	if surface == nil {
		return
	}
	h.inputHooks = true
	// Debounced: size notify storms should not re-stamp dozens of times.
	var pending uint64
	resync := func() {
		id := h.inputGen.Add(1)
		pending = id
		glib.TimeoutAdd(50, func() bool {
			if pending != id {
				return false
			}
			_ = shell.ApplyInputMode(&h.win.Window, h.interactive)
			return false
		})
	}
	surface.Connect("notify::width", func(_ *gdk.Surface) { resync() })
	surface.Connect("notify::height", func(_ *gdk.Surface) { resync() })
}

func (h *Host) scheduleInputSync() {
	gen := h.inputGen.Add(1)
	once := func() bool {
		if gen != h.inputGen.Load() {
			return false
		}
		_ = shell.ApplyInputMode(&h.win.Window, h.interactive)
		return false
	}
	glib.IdleAdd(once)
	for _, ms := range []uint{16, 80, 250} {
		delay := ms
		glib.TimeoutAdd(delay, once)
	}
}
