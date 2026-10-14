package wireguard

import "time"

// Peer is the desired configuration of a WireGuard peer on an interface.
type Peer struct {
	PublicKey    string
	PresharedKey string
	Endpoint     string   // optional, "host:port"
	AllowedIPs   []string // CIDRs routed to this peer, e.g. "10.8.0.5/32"
	Keepalive    int      // seconds, 0 to disable
}

// NewDevicePeer builds the server-side peer entry for a device from its
// tunnel addresses (IPv4, then IPv6 when enabled).
func NewDevicePeer(publicKey string, addresses ...string) Peer {
	allowed := make([]string, 0, len(addresses))
	for _, a := range addresses {
		allowed = append(allowed, hostIPWithCIDR(a))
	}
	return Peer{
		PublicKey:  publicKey,
		AllowedIPs: allowed,
	}
}

// PeerStatus is the runtime state of a peer as reported by the kernel/userspace device.
type PeerStatus struct {
	Peer
	LastHandshake time.Time
	ReceiveBytes  int64
	TransmitBytes int64
}

// IsOnline reports whether the peer completed a handshake within the given window.
func (p PeerStatus) IsOnline(within time.Duration) bool {
	if p.LastHandshake.IsZero() {
		return false
	}
	return time.Since(p.LastHandshake) <= within
}

// InterfaceStatus is the runtime state of a WireGuard interface.
type InterfaceStatus struct {
	Name       string
	PublicKey  string
	ListenPort int
	Peers      []PeerStatus
}
