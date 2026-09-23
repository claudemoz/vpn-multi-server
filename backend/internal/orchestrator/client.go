// Package orchestrator is the control-plane side of the gRPC link with the
// agents. It dials each VPN server's grpc_endpoint, pushes the desired peer
// set and collects the observed state so the database reflects reality.
package orchestrator

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	agentv1 "backend/gen/agent/v1"
	"backend/internal/agent"
	"backend/internal/agentproto"
	"backend/internal/config"
	"backend/internal/wireguard"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ErrAgentUnavailable is returned when the agent cannot be reached.
var ErrAgentUnavailable = errors.New("agent unavailable")

// Health is the agent's self-reported status.
type Health struct {
	Interface   string `json:"interface"`
	Backend     string `json:"backend"`
	Version     string `json:"version"`
	InterfaceUp bool   `json:"interface_up"`
}

// AgentClient is a typed wrapper around the generated gRPC client. It talks in
// wireguard.* types so callers never see protobuf.
type AgentClient struct {
	endpoint    string
	conn        *grpc.ClientConn
	rpc         agentv1.AgentServiceClient
	token       string
	callTimeout time.Duration
}

// Dialer creates AgentClients with the platform-wide settings (auth, TLS,
// timeouts). One Dialer is shared by the Pool.
type Dialer struct {
	cfg   config.AgentConfig
	creds credentials.TransportCredentials
}

func NewDialer(cfg config.AgentConfig) (*Dialer, error) {
	creds, err := transportCredentials(cfg)
	if err != nil {
		return nil, err
	}
	return &Dialer{cfg: cfg, creds: creds}, nil
}

// Dial opens a connection to an agent. The connection is lazy: gRPC connects
// on first use and reconnects transparently afterwards.
func (d *Dialer) Dial(endpoint string) (*AgentClient, error) {
	if endpoint == "" {
		return nil, errors.New("empty agent endpoint")
	}

	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(d.creds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("dial agent %s: %w", endpoint, err)
	}

	return &AgentClient{
		endpoint:    endpoint,
		conn:        conn,
		rpc:         agentv1.NewAgentServiceClient(conn),
		token:       d.cfg.Token,
		callTimeout: d.cfg.CallTimeout,
	}, nil
}

func transportCredentials(cfg config.AgentConfig) (credentials.TransportCredentials, error) {
	if cfg.TLSCAFile == "" {
		return insecure.NewCredentials(), nil
	}

	pem, err := os.ReadFile(cfg.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("read AGENT_TLS_CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("AGENT_TLS_CA contains no valid certificate")
	}

	return credentials.NewTLS(&tls.Config{
		RootCAs:    pool,
		ServerName: cfg.TLSServerName,
		MinVersion: tls.VersionTLS12,
	}), nil
}

func (c *AgentClient) Endpoint() string { return c.endpoint }

func (c *AgentClient) Close() error { return c.conn.Close() }

func (c *AgentClient) Health(ctx context.Context) (*Health, error) {
	ctx, cancel := c.prepare(ctx)
	defer cancel()

	resp, err := c.rpc.Health(ctx, &agentv1.HealthRequest{})
	if err != nil {
		return nil, c.wrap(err)
	}
	return &Health{
		Interface:   resp.GetInterface(),
		Backend:     resp.GetBackend(),
		Version:     resp.GetVersion(),
		InterfaceUp: resp.GetInterfaceUp(),
	}, nil
}

func (c *AgentClient) Interface(ctx context.Context) (*wireguard.InterfaceStatus, error) {
	ctx, cancel := c.prepare(ctx)
	defer cancel()

	resp, err := c.rpc.GetInterface(ctx, &agentv1.GetInterfaceRequest{})
	if err != nil {
		return nil, c.wrap(err)
	}
	return agentproto.InterfaceFromProto(resp.GetInterface()), nil
}

func (c *AgentClient) ListPeers(ctx context.Context) ([]wireguard.PeerStatus, error) {
	ctx, cancel := c.prepare(ctx)
	defer cancel()

	resp, err := c.rpc.ListPeers(ctx, &agentv1.ListPeersRequest{})
	if err != nil {
		return nil, c.wrap(err)
	}
	return agentproto.PeerStatusesFromProto(resp.GetPeers()), nil
}

func (c *AgentClient) AddPeer(ctx context.Context, peer wireguard.Peer) error {
	ctx, cancel := c.prepare(ctx)
	defer cancel()

	_, err := c.rpc.AddPeer(ctx, &agentv1.AddPeerRequest{Peer: agentproto.PeerToProto(peer)})
	return c.wrap(err)
}

func (c *AgentClient) RemovePeer(ctx context.Context, publicKey string) error {
	ctx, cancel := c.prepare(ctx)
	defer cancel()

	_, err := c.rpc.RemovePeer(ctx, &agentv1.RemovePeerRequest{PublicKey: publicKey})
	return c.wrap(err)
}

// SyncPeers replaces the agent's peer set and returns the resulting count.
func (c *AgentClient) SyncPeers(ctx context.Context, peers []wireguard.Peer) (int, error) {
	ctx, cancel := c.prepare(ctx)
	defer cancel()

	resp, err := c.rpc.SyncPeers(ctx, &agentv1.SyncPeersRequest{Peers: agentproto.PeersToProto(peers)})
	if err != nil {
		return 0, c.wrap(err)
	}
	return int(resp.GetPeerCount()), nil
}

// prepare attaches the bearer token and a per-call deadline.
func (c *AgentClient) prepare(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, agent.AuthorizationHeader, "Bearer "+c.token)
	}
	if c.callTimeout > 0 {
		return context.WithTimeout(ctx, c.callTimeout)
	}
	return context.WithCancel(ctx)
}

// wrap turns transport-level failures into ErrAgentUnavailable and keeps the
// gRPC status message for everything else.
func (c *AgentClient) wrap(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("agent %s: %w", c.endpoint, err)
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return fmt.Errorf("%w: %s (%s)", ErrAgentUnavailable, c.endpoint, st.Message())
	default:
		return fmt.Errorf("agent %s: %s: %s", c.endpoint, st.Code(), st.Message())
	}
}
