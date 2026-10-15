//go:build quests_overlay

package stylepack

import (
	"sync"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Manager owns the active pack + CSS provider (swappable at runtime).
type Manager struct {
	mu       sync.Mutex
	theme    Theme
	look     Look
	display  *gdk.Display
	provider *gtk.CSSProvider
}

var global = &Manager{theme: Resolve(DefaultID)}

// Active returns the current theme (copy).
func Active() Theme {
	global.mu.Lock()
	defer global.mu.Unlock()
	return global.theme
}

// ActiveID returns the current pack id.
func ActiveID() string {
	return Active().ID
}

// Apply installs pack CSS for display (replacing previous provider).
func Apply(display *gdk.Display, packID string, look Look) Theme {
	return global.Apply(display, packID, look)
}

// Apply sets pack + look and (re)loads CSS.
func (m *Manager) Apply(display *gdk.Display, packID string, look Look) Theme {
	m.mu.Lock()
	defer m.mu.Unlock()
	if display != nil {
		m.display = display
	}
	m.theme = Resolve(packID)
	m.look = look
	m.reloadLocked()
	return m.theme
}

// SetLook reloads CSS with new passthrough look, keeping pack.
func (m *Manager) SetLook(look Look) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.look = look
	m.reloadLocked()
}

func (m *Manager) reloadLocked() {
	if m.display == nil {
		return
	}
	css := BuildCSS(m.theme, m.look)
	next := gtk.NewCSSProvider()
	next.LoadFromString(css)
	if m.provider != nil {
		gtk.StyleContextRemoveProviderForDisplay(m.display, m.provider)
	}
	gtk.StyleContextAddProviderForDisplay(m.display, next, uint(gtk.STYLE_PROVIDER_PRIORITY_USER))
	m.provider = next
}
