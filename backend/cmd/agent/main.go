// Command agent runs on every WireGuard server. It exposes the gRPC
// AgentService so the control plane can push peers and read their state.
//
// Configuration (environment):
//
//	AGENT_LISTEN            ":50051"   gRPC listen address
//	WG_INTERFACE            "wg0"      interface to manage
//	AGENT_BACKEND           "wgctrl"   "wgctrl" (real) or "memory" (dev)
//	AGENT_TOKEN                        shared secret with the control plane
//	AGENT_INSECURE_NO_AUTH  "false"    skip token check (dev only)
//	AGENT_TLS_CERT / AGENT_TLS_KEY     enable TLS on the listener
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"backend/internal/agent"
)

func main() {
	cfg, err := agent.LoadConfig()
	if err != nil {
		log.Fatalf("agent config: %v", err)
	}

	srv, err := agent.New(cfg)
	if err != nil {
		log.Fatalf("agent init: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("agent: %v", err)
	}
	log.Println("agent stopped")
}
