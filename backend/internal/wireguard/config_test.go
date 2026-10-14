package wireguard

import (
	"strings"
	"testing"
)

func TestRenderClientConfig(t *testing.T) {
	config := RenderClientConfig(ClientConfigInput{
		PrivateKey:      "client-private-key",
		AssignedIP:      "10.8.0.5",
		DNS:             []string{"1.1.1.1"},
		ServerPublicKey: "server-public-key",
		ServerEndpoint:  "vpn.example.com:51820",
		AllowedIPs:      []string{"0.0.0.0/0"},
		Keepalive:       25,
	})

	for _, expected := range []string{
		"PrivateKey = client-private-key",
		"Address = 10.8.0.5/32",
		"DNS = 1.1.1.1",
		"PublicKey = server-public-key",
		"Endpoint = vpn.example.com:51820",
		"AllowedIPs = 0.0.0.0/0",
		"PersistentKeepalive = 25",
	} {
		if !strings.Contains(config, expected) {
			t.Fatalf("expected config to contain %q, got:\n%s", expected, config)
		}
	}
}

func TestSettingsClientConfigMTU(t *testing.T) {
	s, err := NewSettings("10.8.0.0/24", "1.1.1.1", "0.0.0.0/0", 25, 1280, "")
	if err != nil {
		t.Fatal(err)
	}
	config := s.ClientConfig("k", "10.8.0.5", "server-public-key", "vpn.example.com:51820")
	if !strings.Contains(config, "MTU = 1280") {
		t.Fatalf("expected MTU in config, got:\n%s", config)
	}

	s, err = NewSettings("10.8.0.0/24", "1.1.1.1", "0.0.0.0/0", 25, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if config := s.ClientConfig("k", "10.8.0.5", "pk", "vpn.example.com:51820"); strings.Contains(config, "MTU") {
		t.Fatalf("expected no MTU line when mtu is 0, got:\n%s", config)
	}

	if _, err := NewSettings("10.8.0.0/24", "1.1.1.1", "0.0.0.0/0", 25, 1000, ""); err == nil {
		t.Fatal("expected an error for an mtu below 1280")
	}
}
