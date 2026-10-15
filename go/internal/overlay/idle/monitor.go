//go:build quests_overlay

// Package idle waits for seat activity via ext-idle-notify-v1 (major toast AFK hold).
// Pure Go Wayland client — must NOT link libwayland-client (breaks gtk4-layer-shell in-process).
//
// All protocol I/O is driven from the GTK main loop via Roundtrip (no parallel
// dispatch goroutine — that raced WriteMsg/ReadMsg and dropped idled events).
package idle

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MatthiasKunnen/go-wayland/wayland/client"
	extidle "github.com/MatthiasKunnen/go-wayland/wayland/staging/ext-idle-notify-v1"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

const (
	activeProbe = 300 * time.Millisecond
	afkMax      = 15 * time.Minute
	pumpEveryMS = 33
)

// Monitor is a persistent Wayland idle connection (separate from GTK's display).
type Monitor struct {
	mu sync.Mutex

	display  *client.Display
	registry *client.Registry
	seat     *client.Seat
	notifier *extidle.IdleNotifier
	notif    *extidle.IdleNotification

	seatName, seatVer         uint32
	notifierName, notifierVer uint32

	sawIdled atomic.Bool
	token    atomic.Uint64
	onActive func()
	pumping  atomic.Bool
}

var global struct {
	once sync.Once
	m    *Monitor
}

// Get returns the process-wide idle monitor (lazy).
func Get() *Monitor {
	global.once.Do(func() {
		global.m = &Monitor{}
	})
	return global.m
}

// Start connects to the compositor. Safe to call repeatedly.
func (m *Monitor) Start() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.display != nil && m.seat != nil && m.notifier != nil {
		return true
	}
	m.teardownLocked()

	display, err := client.Connect("")
	if err != nil {
		log.Printf("idle-notify: display connect failed: %v", err)
		return false
	}
	m.display = display

	registry, err := display.GetRegistry()
	if err != nil {
		log.Printf("idle-notify: registry failed: %v", err)
		m.teardownLocked()
		return false
	}
	m.registry = registry
	registry.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		switch e.Interface {
		case "wl_seat":
			m.seatName, m.seatVer = e.Name, e.Version
		case extidle.IdleNotifierInterfaceName:
			m.notifierName, m.notifierVer = e.Name, e.Version
		}
	})
	if err := display.Roundtrip(); err != nil {
		log.Printf("idle-notify: roundtrip failed: %v", err)
		m.teardownLocked()
		return false
	}
	if m.seatName == 0 || m.notifierName == 0 {
		log.Printf("idle-notify: missing globals (seat=%d notifier=%d)", m.seatName, m.notifierName)
		m.teardownLocked()
		return false
	}

	seat := client.NewSeat(display.Context())
	if err := registryBind(registry, m.seatName, "wl_seat", min(m.seatVer, 1), seat); err != nil {
		log.Printf("idle-notify: bind seat: %v", err)
		m.teardownLocked()
		return false
	}
	m.seat = seat

	notifier := extidle.NewIdleNotifier(display.Context())
	ver := min(m.notifierVer, 2)
	if ver < 1 {
		ver = 1
	}
	if err := registryBind(registry, m.notifierName, extidle.IdleNotifierInterfaceName, ver, notifier); err != nil {
		log.Printf("idle-notify: bind notifier: %v", err)
		m.teardownLocked()
		return false
	}
	m.notifier = notifier
	m.notifierVer = ver
	if err := display.Roundtrip(); err != nil {
		log.Printf("idle-notify: bind roundtrip: %v", err)
		m.teardownLocked()
		return false
	}

	log.Printf("idle-notify: ready (notifier v%d)", ver)
	return true
}

// Stop tears down the Wayland connection.
func (m *Monitor) Stop() {
	m.CancelWait()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teardownLocked()
}

func (m *Monitor) teardownLocked() {
	m.pumping.Store(false)
	if m.notif != nil {
		_ = m.notif.Destroy()
		m.notif = nil
	}
	if m.notifier != nil {
		_ = m.notifier.Destroy()
		m.notifier = nil
	}
	m.seat = nil
	m.registry = nil
	if m.display != nil {
		ctx := m.display.Context()
		_ = m.display.Destroy()
		_ = ctx.Close()
		m.display = nil
	}
	m.seatName, m.notifierName = 0, 0
}

// CancelWait aborts an in-flight WaitForActivity.
func (m *Monitor) CancelWait() {
	m.token.Add(1)
	m.pumping.Store(false)
	m.mu.Lock()
	m.onActive = nil
	m.sawIdled.Store(false)
	if m.notif != nil {
		_ = m.notif.Destroy()
		m.notif = nil
	}
	m.mu.Unlock()
}

// WaitForActivity invokes onActive when the seat is (or becomes) active.
// After MajorHold: if currently idle → stay until resumed; if active → fire soon.
// Returns false when idle-notify is unavailable (caller should fade immediately).
func (m *Monitor) WaitForActivity(onActive func()) bool {
	if onActive == nil {
		return false
	}
	if !m.Start() {
		return false
	}

	m.CancelWait()
	tok := m.token.Add(1)

	m.mu.Lock()
	if m.notifier == nil || m.seat == nil || m.display == nil {
		m.mu.Unlock()
		return false
	}
	m.onActive = onActive
	m.sawIdled.Store(false)

	var notif *extidle.IdleNotification
	var err error
	if m.notifierVer >= 2 {
		notif, err = getInputIdleNotification(m.notifier, 0, m.seat)
	} else {
		notif, err = m.notifier.GetIdleNotification(0, m.seat)
	}
	if err != nil || notif == nil {
		m.onActive = nil
		m.mu.Unlock()
		log.Printf("idle-notify: get notification: %v", err)
		return false
	}
	m.notif = notif
	notif.SetIdledHandler(func(extidle.IdleNotificationIdledEvent) {
		if m.token.Load() != tok {
			return
		}
		m.sawIdled.Store(true)
		log.Print("idle-notify: idled (holding toast)")
	})
	notif.SetResumedHandler(func(extidle.IdleNotificationResumedEvent) {
		if m.token.Load() != tok {
			return
		}
		log.Print("idle-notify: resumed → dismiss")
		m.fire(tok)
	})
	d := m.display
	m.mu.Unlock()
	// Flush create request outside the lock (handlers may run).
	if d != nil {
		_ = d.Roundtrip()
	}

	m.pumping.Store(true)
	glib.TimeoutAdd(pumpEveryMS, func() bool {
		if m.token.Load() != tok || !m.pumping.Load() {
			return false
		}
		// Must not hold mu across Roundtrip: resumed/idled handlers may call fire(),
		// which needs the same mutex (otherwise the toast stays forever).
		m.mu.Lock()
		d := m.display
		m.mu.Unlock()
		if d != nil {
			_ = d.Roundtrip()
		}
		return true
	})

	glib.TimeoutAdd(uint(activeProbe.Milliseconds()), func() bool {
		if m.token.Load() != tok {
			return false
		}
		if !m.sawIdled.Load() {
			log.Print("idle-notify: seat active → dismiss")
			m.fire(tok)
		}
		return false
	})
	glib.TimeoutAdd(uint(afkMax.Milliseconds()), func() bool {
		if m.token.Load() != tok {
			return false
		}
		log.Print("idle-notify: AFK max reached, dismissing")
		m.fire(tok)
		return false
	})
	return true
}

func (m *Monitor) fire(tok uint64) {
	if !m.token.CompareAndSwap(tok, tok+1) {
		return
	}
	m.pumping.Store(false)
	m.mu.Lock()
	cb := m.onActive
	m.onActive = nil
	if m.notif != nil {
		_ = m.notif.Destroy()
		m.notif = nil
	}
	m.mu.Unlock()
	if cb == nil {
		return
	}
	glib.IdleAdd(func() bool {
		cb()
		return false
	})
}
