package wireguard

import (
	"fmt"
	"net/netip"
)

// ParseIPv6Prefix validates the prefix devices get their IPv6 tunnel address
// from. It must leave at least 32 host bits, since the device IPv4 is
// embedded in the low 32 bits (see DeviceIPv6).
func ParseIPv6Prefix(raw string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("parse ipv6 prefix %q: %w", raw, err)
	}
	if !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("ipv6 prefix %q is not an IPv6 prefix", raw)
	}
	if prefix.Bits() > 96 {
		return netip.Prefix{}, fmt.Errorf("ipv6 prefix %q must be /96 or larger", raw)
	}
	return prefix.Masked(), nil
}

// DeviceIPv6 derives the IPv6 tunnel address of a device from its IPv4: the
// IPv4 fills the low 32 bits of the prefix (10.8.0.5 -> <prefix>::a08:5).
// IPv4 addresses are unique per server, so the IPv6 ones are too, without
// storing or allocating anything.
func DeviceIPv6(prefix netip.Prefix, ipv4 string) (string, error) {
	v4, err := netip.ParseAddr(normalizeHostIP(ipv4))
	if err != nil || !v4.Is4() {
		return "", fmt.Errorf("invalid device ipv4 %q", ipv4)
	}
	b := prefix.Masked().Addr().As16()
	copy(b[12:], v4.AsSlice())
	return netip.AddrFrom16(b).String(), nil
}
