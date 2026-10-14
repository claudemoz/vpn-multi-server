package wireguard

import (
	"fmt"
	"log"
	"net/netip"
)

// Settings groups the platform-wide WireGuard parameters used when
// provisioning devices and rendering client configurations.
type Settings struct {
	Pool       *Pool
	DNS        []string
	AllowedIPs []string
	Keepalive  int
	// MTU of the client tunnel interface; 0 leaves it to the client.
	MTU int
	// IPv6Prefix gives each device an IPv6 tunnel address (see DeviceIPv6).
	// Zero value: IPv4-only tunnels.
	IPv6Prefix netip.Prefix
}

// NewSettings builds Settings from raw configuration values.
// dns and allowedIPs are comma-separated lists; an empty ipv6Prefix disables
// IPv6 inside the tunnel.
func NewSettings(subnet, dns, allowedIPs string, keepalive, mtu int, ipv6Prefix string) (*Settings, error) {
	pool, err := NewPool(subnet)
	if err != nil {
		return nil, fmt.Errorf("wireguard subnet: %w", err)
	}
	// 1280 is the IPv6 minimum; above 1500 the encapsulated packet can never
	// fit a standard link.
	if mtu != 0 && (mtu < 1280 || mtu > 1500) {
		return nil, fmt.Errorf("wireguard mtu %d out of range [1280, 1500]", mtu)
	}
	var prefix netip.Prefix
	if ipv6Prefix != "" {
		if prefix, err = ParseIPv6Prefix(ipv6Prefix); err != nil {
			return nil, fmt.Errorf("wireguard %w", err)
		}
	}

	return &Settings{
		Pool:       pool,
		DNS:        SplitList(dns),
		AllowedIPs: SplitList(allowedIPs),
		Keepalive:  keepalive,
		MTU:        mtu,
		IPv6Prefix: prefix,
	}, nil
}

// DeviceAddresses returns the tunnel addresses of a device: its IPv4, plus
// the derived IPv6 when IPv6 is enabled.
func (s *Settings) DeviceAddresses(ipv4 string) []string {
	addresses := []string{ipv4}
	if v6 := s.deviceIPv6(ipv4); v6 != "" {
		addresses = append(addresses, v6)
	}
	return addresses
}

// DevicePeer builds the server-side peer of a device, with all its addresses.
func (s *Settings) DevicePeer(publicKey, ipv4 string) Peer {
	return NewDevicePeer(publicKey, s.DeviceAddresses(ipv4)...)
}

func (s *Settings) deviceIPv6(ipv4 string) string {
	if !s.IPv6Prefix.IsValid() {
		return ""
	}
	v6, err := DeviceIPv6(s.IPv6Prefix, ipv4)
	if err != nil {
		// Only possible with a corrupted assigned_ip; the tunnel still works
		// in IPv4.
		log.Printf("wireguard: no ipv6 for device ip %q: %v", ipv4, err)
		return ""
	}
	return v6
}

// ClientConfig renders a client configuration for a device on the given server.
func (s *Settings) ClientConfig(privateKey, assignedIP, serverPublicKey, serverEndpoint string) string {
	return RenderClientConfig(ClientConfigInput{
		PrivateKey:      privateKey,
		AssignedIP:      assignedIP,
		AssignedIPv6:    s.deviceIPv6(assignedIP),
		DNS:             s.DNS,
		ServerPublicKey: serverPublicKey,
		ServerEndpoint:  serverEndpoint,
		AllowedIPs:      s.AllowedIPs,
		Keepalive:       s.Keepalive,
		MTU:             s.MTU,
	})
}
