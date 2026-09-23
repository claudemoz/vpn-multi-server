package agent

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	agentv1 "backend/gen/agent/v1"
	"backend/internal/wireguard"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Server wraps the gRPC server, its listener and the wireguard.Client so the
// whole agent can be started and stopped as one unit.
type Server struct {
	cfg  Config
	wg   wireguard.Client
	grpc *grpc.Server
}

// New builds an agent from its configuration: opens the WireGuard backend,
// installs the auth interceptor and optional TLS, and registers the service.
func New(cfg Config) (*Server, error) {
	wg, err := newWireGuardClient(cfg)
	if err != nil {
		return nil, err
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
	}

	if cfg.InsecureNoAuth {
		log.Println("WARNING: AGENT_INSECURE_NO_AUTH=true, every caller is trusted")
	} else {
		opts = append(opts, grpc.ChainUnaryInterceptor(TokenInterceptor(cfg.Token)))
	}

	if cfg.TLSEnabled() {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			_ = wg.Close()
			return nil, fmt.Errorf("load tls credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	} else {
		log.Println("WARNING: TLS disabled, set AGENT_TLS_CERT/AGENT_TLS_KEY in production")
	}

	srv := grpc.NewServer(opts...)
	agentv1.RegisterAgentServiceServer(srv, NewService(wg, cfg.Interface, cfg.Backend))

	return &Server{cfg: cfg, wg: wg, grpc: srv}, nil
}

// Serve listens until ctx is cancelled, then stops gracefully within
// cfg.ShutdownTimeout.
func (s *Server) Serve(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.ListenAddr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("agent listening on %s (interface=%s backend=%s tls=%t)",
			lis.Addr(), s.cfg.Interface, s.cfg.Backend, s.cfg.TLSEnabled())
		errCh <- s.grpc.Serve(lis)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	stopped := make(chan struct{})
	go func() {
		s.grpc.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(s.cfg.ShutdownTimeout):
		s.grpc.Stop()
	}

	return s.wg.Close()
}

func newWireGuardClient(cfg Config) (wireguard.Client, error) {
	switch cfg.Backend {
	case BackendMemory:
		return wireguard.NewMemoryClient(cfg.Interface), nil
	case BackendWGCtrl:
		c, err := wireguard.NewWGCtrlClient()
		if err != nil {
			return nil, err
		}
		// Fail fast if the interface is not there: the agent is useless
		// without it, and the operator wants to know at startup.
		if _, err := c.Interface(context.Background(), cfg.Interface); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("interface %s: %w (run scripts/setup-wg-server.sh first)", cfg.Interface, err)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unknown backend %q", cfg.Backend)
	}
}
