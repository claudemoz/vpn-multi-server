package wireguard

import (
	"reflect"
	"strings"
	"testing"
)

func TestDeviceIPv6(t *testing.T) {
	prefix, err := ParseIPv6Prefix("fd7a:5f1c:9e42::/64")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DeviceIPv6(prefix, "10.8.0.5")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fd7a:5f1c:9e42::a08:5" {
		t.Fatalf("got %q", got)
	}

	if _, err := DeviceIPv6(prefix, "not-an-ip"); err == nil {
		t.Fatal("expected an error for an invalid ipv4")
	}
}

func TestParseIPv6PrefixRejects(t *testing.T) {
	for _, raw := range []string{"10.8.0.0/24", "fd00::/120", "nope"} {
		if _, err := ParseIPv6Prefix(raw); err == nil {
			t.Errorf("expected %q to be rejected", raw)
		}
	}
}

func TestHostIPWithCIDRIPv6(t *testing.T) {
	if got := hostIPWithCIDR("fd7a::a08:5"); got != "fd7a::a08:5/128" {
		t.Fatalf("got %q", got)
	}
	if got := hostIPWithCIDR("10.8.0.5"); got != "10.8.0.5/32" {
		t.Fatalf("got %q", got)
	}
}

func TestSettingsDualStack(t *testing.T) {
	s, err := NewSettings("10.8.0.0/24", "1.1.1.1", "0.0.0.0/0, ::/0", 25, 1280, "fd7a:5f1c:9e42::/64")
	if err != nil {
		t.Fatal(err)
	}

	peer := s.DevicePeer("pk", "10.8.0.5")
	want := []string{"10.8.0.5/32", "fd7a:5f1c:9e42::a08:5/128"}
	if !reflect.DeepEqual(peer.AllowedIPs, want) {
		t.Fatalf("peer allowed ips = %v, want %v", peer.AllowedIPs, want)
	}

	config := s.ClientConfig("k", "10.8.0.5", "server-public-key", "vpn.example.com:51820")
	if !strings.Contains(config, "Address = 10.8.0.5/32, fd7a:5f1c:9e42::a08:5/128") {
		t.Fatalf("expected dual-stack address, got:\n%s", config)
	}

	s, err = NewSettings("10.8.0.0/24", "1.1.1.1", "0.0.0.0/0", 25, 1280, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.DevicePeer("pk", "10.8.0.5").AllowedIPs; !reflect.DeepEqual(got, []string{"10.8.0.5/32"}) {
		t.Fatalf("ipv4-only peer allowed ips = %v", got)
	}
}
