package agentproto

import (
	"reflect"
	"testing"
	"time"

	"backend/internal/wireguard"
)

func TestPeerRoundTrip(t *testing.T) {
	in := wireguard.Peer{
		PublicKey:    "pub",
		PresharedKey: "psk",
		Endpoint:     "1.2.3.4:51820",
		AllowedIPs:   []string{"10.8.0.2/32", "fd00::2/128"},
		Keepalive:    25,
	}

	out := PeerFromProto(PeerToProto(in))
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestPeerStatusRoundTripKeepsHandshake(t *testing.T) {
	hs := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	in := wireguard.PeerStatus{
		Peer:          wireguard.Peer{PublicKey: "pub", AllowedIPs: []string{"10.8.0.2/32"}},
		LastHandshake: hs,
		ReceiveBytes:  10,
		TransmitBytes: 20,
	}

	out := PeerStatusFromProto(PeerStatusToProto(in))
	if !out.LastHandshake.Equal(hs) {
		t.Fatalf("handshake lost: %v", out.LastHandshake)
	}
	if out.ReceiveBytes != 10 || out.TransmitBytes != 20 {
		t.Fatalf("counters mismatch: %+v", out)
	}
}

func TestPeerStatusZeroHandshakeStaysZero(t *testing.T) {
	in := wireguard.PeerStatus{Peer: wireguard.Peer{PublicKey: "pub"}}

	pb := PeerStatusToProto(in)
	if pb.LastHandshake != nil {
		t.Fatal("zero handshake must not be serialised")
	}
	if out := PeerStatusFromProto(pb); !out.LastHandshake.IsZero() {
		t.Fatalf("expected zero handshake, got %v", out.LastHandshake)
	}
}

func TestInterfaceRoundTrip(t *testing.T) {
	in := &wireguard.InterfaceStatus{
		Name:       "wg0",
		PublicKey:  "srv",
		ListenPort: 51820,
		Peers: []wireguard.PeerStatus{
			{Peer: wireguard.Peer{PublicKey: "a", AllowedIPs: []string{"10.8.0.2/32"}}},
		},
	}

	out := InterfaceFromProto(InterfaceToProto(in))
	if out.Name != in.Name || out.PublicKey != in.PublicKey || out.ListenPort != in.ListenPort {
		t.Fatalf("interface mismatch: %+v", out)
	}
	if len(out.Peers) != 1 || out.Peers[0].PublicKey != "a" {
		t.Fatalf("peers mismatch: %+v", out.Peers)
	}
	if InterfaceFromProto(nil) != nil || InterfaceToProto(nil) != nil {
		t.Fatal("nil must map to nil")
	}
}
