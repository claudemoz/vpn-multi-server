package orchestrator

import (
	"time"

	"backend/internal/device"
	vpnserver "backend/internal/vpn-server"
	"backend/internal/wireguard"

	"github.com/google/uuid"
)

// serverSource adapts vpnserver.Service to ServerSource.
type serverSource struct {
	svc *vpnserver.Service
}

// NewServerSource exposes the vpn-server package to the orchestrator.
func NewServerSource(svc *vpnserver.Service) ServerSource {
	return &serverSource{svc: svc}
}

func (s *serverSource) Target(id uuid.UUID) (Target, error) {
	srv, err := s.svc.GetByID(id)
	if err != nil {
		return Target{}, err
	}
	return Target{ID: id, Name: srv.Name, Endpoint: srv.GRPCEndpoint, IsActive: srv.IsActive}, nil
}

func (s *serverSource) ActiveTargets() ([]Target, error) {
	servers, err := s.svc.GetAll()
	if err != nil {
		return nil, err
	}

	targets := make([]Target, 0, len(servers))
	for _, srv := range servers {
		if !srv.IsActive {
			continue
		}
		id, err := uuid.Parse(srv.ID)
		if err != nil {
			continue
		}
		targets = append(targets, Target{ID: id, Name: srv.Name, Endpoint: srv.GRPCEndpoint, IsActive: true})
	}
	return targets, nil
}

func (s *serverSource) SetCurrentPeers(id uuid.UUID, count int) error {
	return s.svc.SetCurrentPeers(id, count)
}

// peerSource adapts device.Repository to PeerSource.
type peerSource struct {
	repo *device.Repository
	wg   *wireguard.Settings
}

// NewPeerSource exposes the device table to the orchestrator.
func NewPeerSource(repo *device.Repository, wg *wireguard.Settings) PeerSource {
	return &peerSource{repo: repo, wg: wg}
}

func (p *peerSource) DesiredPeers(serverID uuid.UUID) ([]wireguard.Peer, error) {
	devices, err := p.repo.FindActiveByServer(serverID)
	if err != nil {
		return nil, err
	}

	peers := make([]wireguard.Peer, 0, len(devices))
	for _, d := range devices {
		peers = append(peers, p.wg.DevicePeer(d.PublicKey, d.AssignedIP))
	}
	return peers, nil
}

func (p *peerSource) RecordHandshakes(serverID uuid.UUID, statuses []wireguard.PeerStatus) error {
	handshakes := make(map[string]time.Time, len(statuses))
	for _, s := range statuses {
		if !s.LastHandshake.IsZero() {
			handshakes[s.PublicKey] = s.LastHandshake
		}
	}
	if len(handshakes) == 0 {
		return nil
	}
	return p.repo.UpdateLastHandshakes(serverID, handshakes)
}
