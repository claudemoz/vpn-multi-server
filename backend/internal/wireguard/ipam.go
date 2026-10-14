package wireguard

import (
	"fmt"
	"net"
	"strings"
)

type Pool struct {
	network *net.IPNet
	gateway net.IP
}

func NewPool(cidr string) (*Pool, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse cidr %q: %w", cidr, err)
	}

	gateway, err := gatewayIP(network)
	if err != nil {
		return nil, err
	}

	return &Pool{
		network: network,
		gateway: gateway,
	}, nil
}

func (p *Pool) Gateway() string {
	return p.gateway.String()
}

// GatewayCIDR returns the gateway address with the subnet prefix (e.g. "10.8.0.1/24"),
// suitable for the server-side Interface Address.
func (p *Pool) GatewayCIDR() string {
	ones, _ := p.network.Mask.Size()
	return fmt.Sprintf("%s/%d", p.gateway.String(), ones)
}

// Network returns the pool subnet in CIDR notation.
func (p *Pool) Network() string {
	return p.network.String()
}

func (p *Pool) Allocate(usedIPs []string) (string, error) {
	reserved := make(map[string]struct{}, len(usedIPs)+2)
	reserved[p.network.IP.String()] = struct{}{}
	reserved[p.gateway.String()] = struct{}{}

	for _, ip := range usedIPs {
		reserved[normalizeHostIP(ip)] = struct{}{}
	}

	ip := cloneIP(p.network.IP)
	for p.network.Contains(ip) {
		candidate := ip.String()
		if _, taken := reserved[candidate]; !taken {
			return candidate, nil
		}
		if !incrementIP(ip) {
			break
		}
	}

	return "", fmt.Errorf("no available ip in subnet %s", p.network.String())
}

func (p *Pool) Contains(ip string) bool {
	parsed := net.ParseIP(normalizeHostIP(ip))
	return parsed != nil && p.network.Contains(parsed)
}

func gatewayIP(network *net.IPNet) (net.IP, error) {
	ip := cloneIP(network.IP)
	if !incrementIP(ip) {
		return nil, fmt.Errorf("subnet %s is too small for gateway", network.String())
	}
	return ip, nil
}

func normalizeHostIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if idx := strings.Index(ip, "/"); idx >= 0 {
		ip = ip[:idx]
	}
	return ip
}

// hostIPWithCIDR turns an address into a single-host CIDR: /32 for IPv4,
// /128 for IPv6.
func hostIPWithCIDR(ip string) string {
	ip = normalizeHostIP(ip)
	if strings.Contains(ip, ":") {
		return ip + "/128"
	}
	return ip + "/32"
}

func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

func incrementIP(ip net.IP) bool {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return true
		}
	}
	return false
}
