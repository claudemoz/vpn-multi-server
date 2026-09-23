package server

import (
	"backend/internal/wireguard"
	apperrors "backend/pkg/errors"

	"github.com/google/uuid"
)

type Service struct {
	repo          *Repository
	defaultSubnet string
}

// NewService creates the server service. defaultSubnet is used when a server is
// created without an explicit subnet.
func NewService(repo *Repository, defaultSubnet string) *Service {
	return &Service{repo: repo, defaultSubnet: defaultSubnet}
}

func (s *Service) GetAll() ([]ServerResponse, error) {
	servers, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	return ToResponseList(servers), nil
}

func (s *Service) GetByID(id uuid.UUID) (ServerResponse, error) {
	server, err := s.repo.FindByID(id)
	if err != nil {
		return ServerResponse{}, err
	}
	return ToResponse(*server), nil
}

func (s *Service) Create(req CreateServerRequest) (ServerResponse, error) {
	if err := wireguard.ValidateKey(req.WGPublicKey); err != nil {
		return ServerResponse{}, apperrors.NewInvalidInput("invalid wg_public_key")
	}

	subnet := req.Subnet
	if subnet == "" {
		subnet = s.defaultSubnet
	}
	if err := validateSubnet(subnet); err != nil {
		return ServerResponse{}, err
	}

	maxPeers := req.MaxPeers
	if maxPeers == 0 {
		maxPeers = 100
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	server := &Server{
		Region:         req.Region,
		Name:           req.Name,
		PublicEndpoint: req.PublicEndpoint,
		WGPublicKey:    req.WGPublicKey,
		GRPCEndpoint:   req.GRPCEndpoint,
		Subnet:         subnet,
		MaxPeers:       maxPeers,
		CurrentPeers:   0,
		IsActive:       isActive,
	}

	if err := s.repo.Create(server); err != nil {
		return ServerResponse{}, err
	}
	return ToResponse(*server), nil
}

func (s *Service) Update(id uuid.UUID, req UpdateServerRequest) (ServerResponse, error) {
	server, err := s.repo.FindByID(id)
	if err != nil {
		return ServerResponse{}, err
	}

	if req.Region != nil {
		server.Region = *req.Region
	}
	if req.Name != nil {
		server.Name = *req.Name
	}
	if req.PublicEndpoint != nil {
		server.PublicEndpoint = *req.PublicEndpoint
	}
	if req.WGPublicKey != nil {
		if err := wireguard.ValidateKey(*req.WGPublicKey); err != nil {
			return ServerResponse{}, apperrors.NewInvalidInput("invalid wg_public_key")
		}
		server.WGPublicKey = *req.WGPublicKey
	}
	if req.GRPCEndpoint != nil {
		server.GRPCEndpoint = *req.GRPCEndpoint
	}
	if req.Subnet != nil {
		if err := validateSubnet(*req.Subnet); err != nil {
			return ServerResponse{}, err
		}
		server.Subnet = *req.Subnet
	}
	if req.MaxPeers != nil {
		server.MaxPeers = *req.MaxPeers
	}
	if req.CurrentPeers != nil {
		server.CurrentPeers = *req.CurrentPeers
	}
	if req.IsActive != nil {
		server.IsActive = *req.IsActive
	}

	if err := s.repo.Update(server); err != nil {
		return ServerResponse{}, err
	}
	return ToResponse(*server), nil
}

func (s *Service) Delete(id uuid.UUID) error {
	return s.repo.Delete(id)
}

// SetCurrentPeers records the peer count reported by the server's agent.
func (s *Service) SetCurrentPeers(id uuid.UUID, count int) error {
	return s.repo.UpdateCurrentPeers(id, count)
}

func (s *Service) Exists(id uuid.UUID) error {
	_, err := s.repo.FindByID(id)
	return err
}

func validateSubnet(subnet string) error {
	if _, err := wireguard.NewPool(subnet); err != nil {
		return apperrors.NewInvalidInput("invalid subnet: " + err.Error())
	}
	return nil
}
