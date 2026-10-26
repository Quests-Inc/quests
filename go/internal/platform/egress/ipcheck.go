package egress

import "net/netip"

// internal lists what is never reached unless the administrator names it in AllowNets.
var internal = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24",
	"192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
	"224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "::ffff:0:0/96", "64:ff9b:1::/48", "100::/64", "2001::/32", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8",
)

func mustPrefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(list))
	for i, s := range list {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

var (
	nat64 = netip.MustParsePrefix("64:ff9b::/96")
	to4   = netip.MustParsePrefix("2002::/16")
)

// Blocked reports whether the address must not be dialled. An IPv4 address wrapped in IPv6 (mapped, NAT64, 6to4) is judged as
// the IPv4 it carries. allow are the administrator's explicit exceptions.
func Blocked(a netip.Addr, allow []netip.Prefix) bool {
	if !a.IsValid() {
		return true
	}
	a = a.WithZone("")
	if a.Is4In6() {
		a = a.Unmap()
	}
	if a.Is6() {
		b := a.As16()
		switch {
		case nat64.Contains(a):
			return Blocked(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), allow)
		case to4.Contains(a):
			return Blocked(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), allow)
		}
	}
	for _, p := range allow {
		if p.Contains(a) {
			return false
		}
	}
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range internal {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
