package wireguard

import (
	"fmt"
	"strings"
)

// PrivateKeyPlaceholder is used when rendering a client config for a device
// whose private key is not known by the platform (client-generated keys).
const PrivateKeyPlaceholder = "<CLIENT_PRIVATE_KEY>"

// ClientConfigInput describes everything needed to render a client wg-quick config.
type ClientConfigInput struct {
	PrivateKey string
	AssignedIP string
	// AssignedIPv6 is optional; when set the interface is dual-stack.
	AssignedIPv6    string
	DNS             []string
	MTU             int
	ServerPublicKey string
	ServerEndpoint  string
	PresharedKey    string
	AllowedIPs      []string
	Keepalive       int
}

// RenderClientConfig renders a wg-quick compatible configuration for a device.
func RenderClientConfig(in ClientConfigInput) string {
	privateKey := in.PrivateKey
	if privateKey == "" {
		privateKey = PrivateKeyPlaceholder
	}

	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", privateKey)
	address := hostIPWithCIDR(in.AssignedIP)
	if in.AssignedIPv6 != "" {
		address += ", " + hostIPWithCIDR(in.AssignedIPv6)
	}
	fmt.Fprintf(&b, "Address = %s\n", address)
	if len(in.DNS) > 0 {
		fmt.Fprintf(&b, "DNS = %s\n", strings.Join(in.DNS, ", "))
	}
	if in.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", in.MTU)
	}

	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", in.ServerPublicKey)
	if in.PresharedKey != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", in.PresharedKey)
	}
	fmt.Fprintf(&b, "Endpoint = %s\n", in.ServerEndpoint)
	if len(in.AllowedIPs) > 0 {
		fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(in.AllowedIPs, ", "))
	}
	if in.Keepalive > 0 {
		fmt.Fprintf(&b, "PersistentKeepalive = %d\n", in.Keepalive)
	}

	return b.String()
}

// ServerConfigInput describes a server-side interface and its peers.
type ServerConfigInput struct {
	PrivateKey string
	Address    string // gateway address with prefix, e.g. "10.8.0.1/24"
	ListenPort int
	MTU        int
	Peers      []Peer
}

// RenderServerConfig renders a server-side wg-quick configuration (wg0.conf).
func RenderServerConfig(in ServerConfigInput) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", in.PrivateKey)
	fmt.Fprintf(&b, "Address = %s\n", in.Address)
	if in.ListenPort > 0 {
		fmt.Fprintf(&b, "ListenPort = %d\n", in.ListenPort)
	}
	if in.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", in.MTU)
	}

	for _, p := range in.Peers {
		b.WriteString("\n[Peer]\n")
		fmt.Fprintf(&b, "PublicKey = %s\n", p.PublicKey)
		if p.PresharedKey != "" {
			fmt.Fprintf(&b, "PresharedKey = %s\n", p.PresharedKey)
		}
		if len(p.AllowedIPs) > 0 {
			fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(p.AllowedIPs, ", "))
		}
		if p.Endpoint != "" {
			fmt.Fprintf(&b, "Endpoint = %s\n", p.Endpoint)
		}
		if p.Keepalive > 0 {
			fmt.Fprintf(&b, "PersistentKeepalive = %d\n", p.Keepalive)
		}
	}

	return b.String()
}

// SplitList parses a comma-separated list (e.g. "1.1.1.1, 8.8.8.8") into trimmed items.
func SplitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
