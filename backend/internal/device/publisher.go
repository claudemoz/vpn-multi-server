package device

import (
	"context"
	"log"

	"backend/internal/wireguard"

	"github.com/google/uuid"
)

// PeerPublisher pushes peer changes to the agent of a VPN server. The
// orchestrator package implements it; the device service only knows this
// interface so the two packages stay decoupled.
type PeerPublisher interface {
	AddPeer(ctx context.Context, serverID uuid.UUID, peer wireguard.Peer) error
	RemovePeer(ctx context.Context, serverID uuid.UUID, publicKey string) error
}

// noopPublisher is used when no agent link is configured (tests, or an API
// running without any agents yet).
type noopPublisher struct{}

func (noopPublisher) AddPeer(context.Context, uuid.UUID, wireguard.Peer) error { return nil }
func (noopPublisher) RemovePeer(context.Context, uuid.UUID, string) error      { return nil }

// publishAdd pushes a device to its server and reports whether it succeeded.
// Failures are logged, not returned: the database is the source of truth and
// the reconciler converges the agent on its next pass.
func (s *Service) publishAdd(ctx context.Context, d *Device) bool {
	peer := wireguard.NewDevicePeer(d.PublicKey, d.AssignedIP)
	if err := s.publisher.AddPeer(ctx, d.ServerID, peer); err != nil {
		log.Printf("device %s: push peer to server %s failed: %v", d.ID, d.ServerID, err)
		return false
	}
	return true
}

func (s *Service) publishRemove(ctx context.Context, serverID uuid.UUID, publicKey string) {
	if err := s.publisher.RemovePeer(ctx, serverID, publicKey); err != nil {
		log.Printf("device %s: remove peer from server %s failed: %v", publicKey, serverID, err)
	}
}
