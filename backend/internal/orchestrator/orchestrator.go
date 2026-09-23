package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/wireguard"

	"github.com/google/uuid"
)

// ErrNoEndpoint is returned when a server has no grpc_endpoint configured.
var ErrNoEndpoint = errors.New("server has no grpc endpoint")

// Target is a VPN server as seen by the orchestrator.
type Target struct {
	ID       uuid.UUID
	Name     string
	Endpoint string
	IsActive bool
}

// ServerSource lists the VPN servers to drive. Implemented on top of the
// vpn-server package (see adapters.go).
type ServerSource interface {
	// Target returns a single server, active or not.
	Target(id uuid.UUID) (Target, error)
	// ActiveTargets returns the servers the reconciler must keep in sync.
	ActiveTargets() ([]Target, error)
	// SetCurrentPeers stores the peer count observed on the agent.
	SetCurrentPeers(id uuid.UUID, count int) error
}

// PeerSource exposes the desired peers of a server and records what the
// agent observed. Implemented on top of the device package (see adapters.go).
type PeerSource interface {
	// DesiredPeers returns the peers that must exist on the server: one per
	// non-revoked, non-deleted device.
	DesiredPeers(serverID uuid.UUID) ([]wireguard.Peer, error)
	// RecordHandshakes updates devices' last_handshake from agent statuses.
	RecordHandshakes(serverID uuid.UUID, statuses []wireguard.PeerStatus) error
}

// Orchestrator drives WireGuard on every VPN server through its agent.
type Orchestrator struct {
	pool    *Pool
	servers ServerSource
	peers   PeerSource
}

func New(pool *Pool, servers ServerSource, peers PeerSource) *Orchestrator {
	return &Orchestrator{pool: pool, servers: servers, peers: peers}
}

// Close releases every agent connection.
func (o *Orchestrator) Close() error { return o.pool.Close() }

// AddPeer pushes a single peer to the server's agent.
func (o *Orchestrator) AddPeer(ctx context.Context, serverID uuid.UUID, peer wireguard.Peer) error {
	client, err := o.clientFor(serverID)
	if err != nil {
		return err
	}
	return client.AddPeer(ctx, peer)
}

// RemovePeer removes a peer from the server's agent.
func (o *Orchestrator) RemovePeer(ctx context.Context, serverID uuid.UUID, publicKey string) error {
	client, err := o.clientFor(serverID)
	if err != nil {
		return err
	}
	return client.RemovePeer(ctx, publicKey)
}

// SyncResult summarises one reconciliation of a server.
type SyncResult struct {
	ServerID  uuid.UUID `json:"server_id"`
	Endpoint  string    `json:"endpoint"`
	PeerCount int       `json:"peer_count"`
	Online    int       `json:"online_peers"`
	Duration  string    `json:"duration"`
}

// OnlineWindow is how recent a handshake must be for a peer to count as online.
const OnlineWindow = 3 * time.Minute

// SyncServer makes the agent's peer set match the database, then reads back
// the peer statuses to update handshakes and the server's current_peers.
func (o *Orchestrator) SyncServer(ctx context.Context, serverID uuid.UUID) (*SyncResult, error) {
	start := time.Now()

	target, err := o.servers.Target(serverID)
	if err != nil {
		return nil, err
	}
	client, err := o.clientForTarget(target)
	if err != nil {
		return nil, err
	}

	desired, err := o.peers.DesiredPeers(serverID)
	if err != nil {
		return nil, fmt.Errorf("load desired peers: %w", err)
	}

	count, err := client.SyncPeers(ctx, desired)
	if err != nil {
		return nil, err
	}

	statuses, err := client.ListPeers(ctx)
	if err != nil {
		return nil, err
	}

	if err := o.peers.RecordHandshakes(serverID, statuses); err != nil {
		return nil, fmt.Errorf("record handshakes: %w", err)
	}
	if err := o.servers.SetCurrentPeers(serverID, count); err != nil {
		return nil, fmt.Errorf("update current_peers: %w", err)
	}

	online := 0
	for _, s := range statuses {
		if s.IsOnline(OnlineWindow) {
			online++
		}
	}

	return &SyncResult{
		ServerID:  serverID,
		Endpoint:  target.Endpoint,
		PeerCount: count,
		Online:    online,
		Duration:  time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

// ServerStatus is the live view of a server as reported by its agent.
type ServerStatus struct {
	ServerID  uuid.UUID                  `json:"server_id"`
	Endpoint  string                     `json:"endpoint"`
	Health    *Health                    `json:"health"`
	Interface *wireguard.InterfaceStatus `json:"interface,omitempty"`
	Online    int                        `json:"online_peers"`
}

// Status queries the agent for its health and interface state.
func (o *Orchestrator) Status(ctx context.Context, serverID uuid.UUID) (*ServerStatus, error) {
	target, err := o.servers.Target(serverID)
	if err != nil {
		return nil, err
	}
	client, err := o.clientForTarget(target)
	if err != nil {
		return nil, err
	}

	health, err := client.Health(ctx)
	if err != nil {
		return nil, err
	}

	out := &ServerStatus{ServerID: serverID, Endpoint: target.Endpoint, Health: health}
	if !health.InterfaceUp {
		return out, nil
	}

	iface, err := client.Interface(ctx)
	if err != nil {
		return nil, err
	}
	out.Interface = iface
	for _, p := range iface.Peers {
		if p.IsOnline(OnlineWindow) {
			out.Online++
		}
	}
	return out, nil
}

// LivePeers returns the peers currently configured on the agent.
func (o *Orchestrator) LivePeers(ctx context.Context, serverID uuid.UUID) ([]wireguard.PeerStatus, error) {
	client, err := o.clientFor(serverID)
	if err != nil {
		return nil, err
	}
	return client.ListPeers(ctx)
}

func (o *Orchestrator) clientFor(serverID uuid.UUID) (*AgentClient, error) {
	target, err := o.servers.Target(serverID)
	if err != nil {
		return nil, err
	}
	return o.clientForTarget(target)
}

func (o *Orchestrator) clientForTarget(t Target) (*AgentClient, error) {
	if t.Endpoint == "" {
		return nil, fmt.Errorf("%w: %s", ErrNoEndpoint, t.ID)
	}
	return o.pool.Get(t.Endpoint)
}
