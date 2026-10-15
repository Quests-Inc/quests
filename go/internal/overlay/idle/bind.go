//go:build quests_overlay

package idle

import (
	"github.com/MatthiasKunnen/go-wayland/wayland/client"
	extidle "github.com/MatthiasKunnen/go-wayland/wayland/staging/ext-idle-notify-v1"
)

// registryBind is wl_registry.bind with a correct Wayland string encoding.
// Upstream client.Registry.Bind passes the *padded* byte length as the string
// length prefix; that works for "wl_seat" (8) but rejects "ext_idle_notifier_v1"
// (21 → pad 24) with "invalid arguments for wl_registry.bind".
func registryBind(reg *client.Registry, name uint32, iface string, version uint32, proxy client.Proxy) error {
	strLen := len(iface) + 1 // include NUL
	padded := client.PaddedLen(strLen)
	bufLen := 8 + 4 + (4 + padded) + 4 + 4
	buf := make([]byte, bufLen)
	l := 0
	client.PutUint32(buf[l:4], reg.ID())
	l += 4
	client.PutUint32(buf[l:l+4], uint32(bufLen<<16|0)) // opcode bind=0
	l += 4
	client.PutUint32(buf[l:l+4], name)
	l += 4
	client.PutUint32(buf[l:l+4], uint32(strLen))
	l += 4
	copy(buf[l:], iface) // trailing NUL + pad already zeroed
	l += padded
	client.PutUint32(buf[l:l+4], version)
	l += 4
	client.PutUint32(buf[l:l+4], proxy.ID())
	return reg.Context().WriteMsg(buf, nil)
}

// getInputIdleNotification is ext_idle_notifier_v1.get_input_idle_notification (v2).
// Ignores idle inhibitors (video etc.) — only real keyboard/pointer activity.
func getInputIdleNotification(notifier *extidle.IdleNotifier, timeout uint32, seat *client.Seat) (*extidle.IdleNotification, error) {
	id := extidle.NewIdleNotification(notifier.Context())
	const opcode = 2
	const reqLen = 8 + 4 + 4 + 4
	var buf [reqLen]byte
	l := 0
	client.PutUint32(buf[l:4], notifier.ID())
	l += 4
	client.PutUint32(buf[l:l+4], uint32(reqLen<<16|opcode&0xffff))
	l += 4
	client.PutUint32(buf[l:l+4], id.ID())
	l += 4
	client.PutUint32(buf[l:l+4], timeout)
	l += 4
	client.PutUint32(buf[l:l+4], seat.ID())
	err := notifier.Context().WriteMsg(buf[:], nil)
	return id, err
}
