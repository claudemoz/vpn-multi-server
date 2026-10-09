package device

import (
	"context"

	vpnserver "backend/internal/vpn-server"
	"backend/internal/wireguard"
	apperrors "backend/pkg/errors"

	"github.com/google/uuid"
)

// ServerProvider exposes the VPN servers a device can be attached to.
type ServerProvider interface {
	GetByID(id uuid.UUID) (vpnserver.ServerResponse, error)
}

type Service struct {
	repo      *Repository
	servers   ServerProvider
	wg        *wireguard.Settings
	publisher PeerPublisher
}

// NewService creates the device service. publisher may be nil when the API
// runs without agents; peer changes are then only persisted.
func NewService(repo *Repository, servers ServerProvider, wg *wireguard.Settings, publisher PeerPublisher) *Service {
	if publisher == nil {
		publisher = noopPublisher{}
	}
	return &Service{repo: repo, servers: servers, wg: wg, publisher: publisher}
}

func (s *Service) GetAll() ([]DeviceResponse, error) {
	devices, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	return ToResponseList(devices), nil
}

func (s *Service) GetByID(id uuid.UUID) (DeviceResponse, error) {
	device, err := s.repo.FindByID(id)
	if err != nil {
		return DeviceResponse{}, err
	}
	return ToResponse(*device), nil
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, req CreateDeviceRequest) (CreateDeviceResponse, error) {
	server, err := s.resolveServer(req.ServerID)
	if err != nil {
		return CreateDeviceResponse{}, err
	}

	publicKey, privateKey := req.PublicKey, ""
	if publicKey == "" {
		kp, err := wireguard.GenerateKeyPair()
		if err != nil {
			return CreateDeviceResponse{}, err
		}
		publicKey, privateKey = kp.PublicKey, kp.PrivateKey
	} else if err := wireguard.ValidateKey(publicKey); err != nil {
		return CreateDeviceResponse{}, apperrors.NewInvalidInput("invalid public_key")
	}

	assignedIP, err := s.pickIP(server, req.AssignedIP)
	if err != nil {
		return CreateDeviceResponse{}, err
	}

	device := &Device{
		UserID:     userID,
		ServerID:   server.id,
		Name:       req.Name,
		PublicKey:  publicKey,
		AssignedIP: assignedIP,
	}

	if err := s.repo.Create(device); err != nil {
		return CreateDeviceResponse{}, err
	}

	synced := s.publishAdd(ctx, device)

	return CreateDeviceResponse{
		DeviceResponse: ToResponse(*device),
		PrivateKey:     privateKey,
		Config:         s.wg.ClientConfig(privateKey, device.AssignedIP, server.WGPublicKey, server.PublicEndpoint),
		AgentSynced:    synced,
	}, nil
}

// ClientConfig renders the wg-quick configuration for an existing device.
// The private key is never stored, so it is rendered as a placeholder.
func (s *Service) ClientConfig(id uuid.UUID) (string, error) {
	device, err := s.repo.FindByID(id)
	if err != nil {
		return "", err
	}

	server, err := s.servers.GetByID(device.ServerID)
	if err != nil {
		return "", err
	}

	return s.wg.ClientConfig("", device.AssignedIP, server.WGPublicKey, server.PublicEndpoint), nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateDeviceRequest) (DeviceResponse, error) {
	device, err := s.repo.FindByID(id)
	if err != nil {
		return DeviceResponse{}, err
	}

	// Remember the peer as currently configured on its agent so we can
	// remove it if the key, IP or server changes.
	prevServerID, prevPublicKey, prevIP := device.ServerID, device.PublicKey, device.AssignedIP

	serverChanged := req.ServerID != nil && *req.ServerID != device.ServerID.String()

	if req.Name != nil {
		device.Name = *req.Name
	}
	if req.PublicKey != nil {
		if err := wireguard.ValidateKey(*req.PublicKey); err != nil {
			return DeviceResponse{}, apperrors.NewInvalidInput("invalid public_key")
		}
		device.PublicKey = *req.PublicKey
	}

	// Any change touching the server or the IP is validated against the
	// target server's subnet; moving to another server re-allocates the IP
	// unless one is explicitly provided.
	if serverChanged || req.AssignedIP != nil {
		targetID := device.ServerID.String()
		if req.ServerID != nil {
			targetID = *req.ServerID
		}
		server, err := s.resolveServer(targetID)
		if err != nil {
			return DeviceResponse{}, err
		}

		requested := ""
		if req.AssignedIP != nil {
			requested = *req.AssignedIP
		} else if !serverChanged {
			requested = device.AssignedIP
		}

		ip, err := s.pickIP(server, requested)
		if err != nil {
			return DeviceResponse{}, err
		}

		device.ServerID = server.id
		device.AssignedIP = ip
	}

	if err := s.repo.Update(device); err != nil {
		return DeviceResponse{}, err
	}

	peerChanged := device.ServerID != prevServerID ||
		device.PublicKey != prevPublicKey ||
		device.AssignedIP != prevIP

	if peerChanged && !device.IsRevoked() {
		// A key or server change leaves a stale peer behind; drop it before
		// pushing the new one. Same server + same key means AddPeer simply
		// replaces the AllowedIPs, so no removal is needed.
		if device.ServerID != prevServerID || device.PublicKey != prevPublicKey {
			s.publishRemove(ctx, prevServerID, prevPublicKey)
		}
		s.publishAdd(ctx, device)
	}

	return ToResponse(*device), nil
}

// Delete removes the device and its peer, which frees its IP on the server,
// and closes its open session.
func (s *Service) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	devices, err := s.repo.FindByUserID(userID)
	if err != nil {
		return err
	}
	for _, device := range devices {
		s.publishRemove(ctx, device.ServerID, device.PublicKey)
	}
	return s.repo.DeleteAllForUser(userID)
}

func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	device, err := s.findOwned(userID, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(id); err != nil {
		return err
	}

	s.publishRemove(ctx, device.ServerID, device.PublicKey)
	return nil
}

// StartSession records that the device's tunnel is up. Calling it again while
// the session is open returns that session unchanged.
func (s *Service) StartSession(userID, id uuid.UUID) (SessionResponse, error) {
	device, err := s.findOwned(userID, id)
	if err != nil {
		return SessionResponse{}, err
	}

	session, err := s.repo.StartSession(device)
	if err != nil {
		return SessionResponse{}, err
	}
	return ToSessionResponse(*session), nil
}

// CurrentSession returns the open session of the device.
func (s *Service) CurrentSession(userID, id uuid.UUID) (SessionResponse, error) {
	if _, err := s.findOwned(userID, id); err != nil {
		return SessionResponse{}, err
	}

	session, err := s.repo.FindOpenSession(id)
	if err != nil {
		return SessionResponse{}, err
	}
	return ToSessionResponse(*session), nil
}

func (s *Service) findOwned(userID, id uuid.UUID) (*Device, error) {
	device, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if device.UserID != userID {
		return nil, apperrors.NewForbidden("device belongs to another user")
	}
	return device, nil
}

type resolvedServer struct {
	vpnserver.ServerResponse
	id   uuid.UUID
	pool *wireguard.Pool
}

func (s *Service) resolveServer(raw string) (resolvedServer, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return resolvedServer{}, apperrors.NewInvalidInput("invalid server_id")
	}

	server, err := s.servers.GetByID(id)
	if err != nil {
		return resolvedServer{}, err
	}
	if !server.IsActive {
		return resolvedServer{}, apperrors.NewInvalidInput("server is not active")
	}

	pool, err := wireguard.NewPool(server.Subnet)
	if err != nil {
		return resolvedServer{}, apperrors.NewInvalidInput("server has an invalid subnet: " + server.Subnet)
	}

	return resolvedServer{ServerResponse: server, id: id, pool: pool}, nil
}

// pickIP validates the requested IP against the server subnet, or allocates
// a free one from the server pool when requested is empty.
func (s *Service) pickIP(server resolvedServer, requested string) (string, error) {
	if requested != "" {
		if !server.pool.Contains(requested) {
			return "", apperrors.NewInvalidInput("assigned_ip is outside the server subnet " + server.pool.Network())
		}
		return requested, nil
	}

	used, err := s.repo.FindAssignedIPsByServer(server.id)
	if err != nil {
		return "", err
	}

	ip, err := server.pool.Allocate(used)
	if err != nil {
		return "", apperrors.NewConflict("no available ip address in " + server.pool.Network())
	}
	return ip, nil
}
