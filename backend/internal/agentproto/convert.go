// Package agentproto converts between the domain types of internal/wireguard
// and the protobuf messages of gen/agent/v1. It is shared by the agent (gRPC
// server) and the control plane (gRPC client) so both sides agree on the
// mapping.
package agentproto

import (
	agentv1 "backend/gen/agent/v1"
	"backend/internal/wireguard"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func PeerToProto(p wireguard.Peer) *agentv1.Peer {
	return &agentv1.Peer{
		PublicKey:    p.PublicKey,
		PresharedKey: p.PresharedKey,
		Endpoint:     p.Endpoint,
		AllowedIps:   append([]string(nil), p.AllowedIPs...),
		Keepalive:    int32(p.Keepalive),
	}
}

func PeerFromProto(p *agentv1.Peer) wireguard.Peer {
	if p == nil {
		return wireguard.Peer{}
	}
	return wireguard.Peer{
		PublicKey:    p.GetPublicKey(),
		PresharedKey: p.GetPresharedKey(),
		Endpoint:     p.GetEndpoint(),
		AllowedIPs:   append([]string(nil), p.GetAllowedIps()...),
		Keepalive:    int(p.GetKeepalive()),
	}
}

func PeersToProto(peers []wireguard.Peer) []*agentv1.Peer {
	out := make([]*agentv1.Peer, 0, len(peers))
	for _, p := range peers {
		out = append(out, PeerToProto(p))
	}
	return out
}

func PeersFromProto(peers []*agentv1.Peer) []wireguard.Peer {
	out := make([]wireguard.Peer, 0, len(peers))
	for _, p := range peers {
		out = append(out, PeerFromProto(p))
	}
	return out
}

func PeerStatusToProto(s wireguard.PeerStatus) *agentv1.PeerStatus {
	out := &agentv1.PeerStatus{
		Peer:          PeerToProto(s.Peer),
		ReceiveBytes:  s.ReceiveBytes,
		TransmitBytes: s.TransmitBytes,
	}
	if !s.LastHandshake.IsZero() {
		out.LastHandshake = timestamppb.New(s.LastHandshake)
	}
	return out
}

func PeerStatusFromProto(s *agentv1.PeerStatus) wireguard.PeerStatus {
	if s == nil {
		return wireguard.PeerStatus{}
	}
	out := wireguard.PeerStatus{
		Peer:          PeerFromProto(s.GetPeer()),
		ReceiveBytes:  s.GetReceiveBytes(),
		TransmitBytes: s.GetTransmitBytes(),
	}
	if ts := s.GetLastHandshake(); ts != nil && ts.IsValid() {
		out.LastHandshake = ts.AsTime()
	}
	return out
}

func PeerStatusesToProto(statuses []wireguard.PeerStatus) []*agentv1.PeerStatus {
	out := make([]*agentv1.PeerStatus, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, PeerStatusToProto(s))
	}
	return out
}

func PeerStatusesFromProto(statuses []*agentv1.PeerStatus) []wireguard.PeerStatus {
	out := make([]wireguard.PeerStatus, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, PeerStatusFromProto(s))
	}
	return out
}

func InterfaceToProto(i *wireguard.InterfaceStatus) *agentv1.InterfaceStatus {
	if i == nil {
		return nil
	}
	return &agentv1.InterfaceStatus{
		Name:       i.Name,
		PublicKey:  i.PublicKey,
		ListenPort: int32(i.ListenPort),
		Peers:      PeerStatusesToProto(i.Peers),
	}
}

func InterfaceFromProto(i *agentv1.InterfaceStatus) *wireguard.InterfaceStatus {
	if i == nil {
		return nil
	}
	return &wireguard.InterfaceStatus{
		Name:       i.GetName(),
		PublicKey:  i.GetPublicKey(),
		ListenPort: int(i.GetListenPort()),
		Peers:      PeerStatusesFromProto(i.GetPeers()),
	}
}
