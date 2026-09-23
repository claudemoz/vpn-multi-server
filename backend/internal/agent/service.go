package agent

import (
	"context"
	"errors"

	agentv1 "backend/gen/agent/v1"
	"backend/internal/agentproto"
	"backend/internal/wireguard"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Version is stamped at build time with -ldflags "-X backend/internal/agent.Version=…".
var Version = "dev"

// Service implements agentv1.AgentServiceServer on top of a wireguard.Client.
// It is deliberately thin: validation and state live in the wireguard package.
type Service struct {
	agentv1.UnimplementedAgentServiceServer

	wg      wireguard.Client
	iface   string
	backend string
}

func NewService(wg wireguard.Client, iface, backend string) *Service {
	return &Service{wg: wg, iface: iface, backend: backend}
}

func (s *Service) Health(ctx context.Context, _ *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	_, err := s.wg.Interface(ctx, s.iface)
	return &agentv1.HealthResponse{
		Interface:   s.iface,
		Backend:     s.backend,
		Version:     Version,
		InterfaceUp: err == nil,
	}, nil
}

func (s *Service) GetInterface(ctx context.Context, _ *agentv1.GetInterfaceRequest) (*agentv1.GetInterfaceResponse, error) {
	iface, err := s.wg.Interface(ctx, s.iface)
	if err != nil {
		return nil, toStatus(err)
	}
	return &agentv1.GetInterfaceResponse{Interface: agentproto.InterfaceToProto(iface)}, nil
}

func (s *Service) ListPeers(ctx context.Context, _ *agentv1.ListPeersRequest) (*agentv1.ListPeersResponse, error) {
	peers, err := s.wg.ListPeers(ctx, s.iface)
	if err != nil {
		return nil, toStatus(err)
	}
	return &agentv1.ListPeersResponse{Peers: agentproto.PeerStatusesToProto(peers)}, nil
}

func (s *Service) AddPeer(ctx context.Context, req *agentv1.AddPeerRequest) (*agentv1.AddPeerResponse, error) {
	if req.GetPeer() == nil {
		return nil, status.Error(codes.InvalidArgument, "peer is required")
	}
	peer := agentproto.PeerFromProto(req.GetPeer())
	if err := wireguard.ValidateKey(peer.PublicKey); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "public_key: %v", err)
	}
	if len(peer.AllowedIPs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "allowed_ips is required")
	}

	if err := s.wg.AddPeer(ctx, s.iface, peer); err != nil {
		return nil, toStatus(err)
	}
	return &agentv1.AddPeerResponse{}, nil
}

func (s *Service) RemovePeer(ctx context.Context, req *agentv1.RemovePeerRequest) (*agentv1.RemovePeerResponse, error) {
	if err := wireguard.ValidateKey(req.GetPublicKey()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "public_key: %v", err)
	}

	if err := s.wg.RemovePeer(ctx, s.iface, req.GetPublicKey()); err != nil {
		return nil, toStatus(err)
	}
	return &agentv1.RemovePeerResponse{}, nil
}

func (s *Service) SyncPeers(ctx context.Context, req *agentv1.SyncPeersRequest) (*agentv1.SyncPeersResponse, error) {
	peers := agentproto.PeersFromProto(req.GetPeers())
	for i, p := range peers {
		if err := wireguard.ValidateKey(p.PublicKey); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "peers[%d].public_key: %v", i, err)
		}
		if len(p.AllowedIPs) == 0 {
			return nil, status.Errorf(codes.InvalidArgument, "peers[%d].allowed_ips is required", i)
		}
	}

	if err := s.wg.SyncPeers(ctx, s.iface, peers); err != nil {
		return nil, toStatus(err)
	}
	return &agentv1.SyncPeersResponse{PeerCount: int32(len(peers))}, nil
}

// toStatus maps wireguard errors to gRPC status codes so the control plane
// can distinguish "interface missing" from "bad input" from "internal".
func toStatus(err error) error {
	switch {
	case errors.Is(err, wireguard.ErrInterfaceNotFound):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, wireguard.ErrInvalidKey):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
