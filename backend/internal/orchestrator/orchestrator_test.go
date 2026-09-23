package orchestrator

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	agentv1 "backend/gen/agent/v1"
	"backend/internal/agent"
	"backend/internal/config"
	"backend/internal/wireguard"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

const (
	testIface = "wg0"
	testToken = "s3cret"
)

// startAgent runs an in-process agent backed by a MemoryClient and returns
// its address plus the memory client so tests can assert on the final state.
func startAgent(t *testing.T) (string, *wireguard.MemoryClient) {
	t.Helper()

	mem := wireguard.NewMemoryClient(testIface)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(agent.TokenInterceptor(testToken)))
	agentv1.RegisterAgentServiceServer(srv, agent.NewService(mem, testIface, agent.BackendMemory))

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return lis.Addr().String(), mem
}

func newTestDialer(t *testing.T, token string) *Dialer {
	t.Helper()
	d, err := NewDialer(config.AgentConfig{Token: token, CallTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("dialer: %v", err)
	}
	return d
}

// fakeServers / fakePeers stand in for the database.
type fakeServers struct {
	mu      sync.Mutex
	targets map[uuid.UUID]Target
	counts  map[uuid.UUID]int
}

func (f *fakeServers) Target(id uuid.UUID) (Target, error) {
	t, ok := f.targets[id]
	if !ok {
		return Target{}, errors.New("server not found")
	}
	return t, nil
}

func (f *fakeServers) ActiveTargets() ([]Target, error) {
	out := make([]Target, 0, len(f.targets))
	for _, t := range f.targets {
		if t.IsActive {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeServers) SetCurrentPeers(id uuid.UUID, count int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[id] = count
	return nil
}

type fakePeers struct {
	mu         sync.Mutex
	desired    map[uuid.UUID][]wireguard.Peer
	handshakes map[uuid.UUID][]wireguard.PeerStatus
}

func (f *fakePeers) DesiredPeers(serverID uuid.UUID) ([]wireguard.Peer, error) {
	return f.desired[serverID], nil
}

func (f *fakePeers) RecordHandshakes(serverID uuid.UUID, statuses []wireguard.PeerStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handshakes[serverID] = statuses
	return nil
}

func mustKey(t *testing.T) string {
	t.Helper()
	kp, err := wireguard.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return kp.PublicKey
}

func TestAgentRejectsBadToken(t *testing.T) {
	addr, _ := startAgent(t)

	client, err := newTestDialer(t, "wrong").Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.Health(context.Background()); err == nil {
		t.Fatal("expected Unauthenticated error, got nil")
	}
}

func TestAddAndRemovePeerThroughOrchestrator(t *testing.T) {
	addr, mem := startAgent(t)
	serverID := uuid.New()

	servers := &fakeServers{
		targets: map[uuid.UUID]Target{serverID: {ID: serverID, Name: "fr-1", Endpoint: addr, IsActive: true}},
		counts:  map[uuid.UUID]int{},
	}
	orch := New(NewPool(newTestDialer(t, testToken)), servers, &fakePeers{})
	defer orch.Close()

	ctx := context.Background()
	key := mustKey(t)

	if err := orch.AddPeer(ctx, serverID, wireguard.NewDevicePeer(key, "10.8.0.2")); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	if got := mem.PublicKeys(testIface); len(got) != 1 || got[0] != key {
		t.Fatalf("agent state after AddPeer: %v", got)
	}

	live, err := orch.LivePeers(ctx, serverID)
	if err != nil {
		t.Fatalf("LivePeers: %v", err)
	}
	if len(live) != 1 || live[0].AllowedIPs[0] != "10.8.0.2/32" {
		t.Fatalf("unexpected live peers: %+v", live)
	}

	if err := orch.RemovePeer(ctx, serverID, key); err != nil {
		t.Fatalf("RemovePeer: %v", err)
	}
	if got := mem.PublicKeys(testIface); len(got) != 0 {
		t.Fatalf("peer still present after RemovePeer: %v", got)
	}
}

func TestSyncServerConvergesAgentAndRecordsState(t *testing.T) {
	addr, mem := startAgent(t)
	serverID := uuid.New()
	ctx := context.Background()

	// Pre-existing stale peer on the agent that is not in the database.
	stale := mustKey(t)
	if err := mem.AddPeer(ctx, testIface, wireguard.NewDevicePeer(stale, "10.8.0.99")); err != nil {
		t.Fatal(err)
	}

	keyA, keyB := mustKey(t), mustKey(t)
	servers := &fakeServers{
		targets: map[uuid.UUID]Target{serverID: {ID: serverID, Name: "fr-1", Endpoint: addr, IsActive: true}},
		counts:  map[uuid.UUID]int{},
	}
	peers := &fakePeers{
		desired: map[uuid.UUID][]wireguard.Peer{serverID: {
			wireguard.NewDevicePeer(keyA, "10.8.0.2"),
			wireguard.NewDevicePeer(keyB, "10.8.0.3"),
		}},
		handshakes: map[uuid.UUID][]wireguard.PeerStatus{},
	}

	orch := New(NewPool(newTestDialer(t, testToken)), servers, peers)
	defer orch.Close()

	res, err := orch.SyncServer(ctx, serverID)
	if err != nil {
		t.Fatalf("SyncServer: %v", err)
	}
	if res.PeerCount != 2 {
		t.Fatalf("expected 2 peers, got %d", res.PeerCount)
	}

	got := mem.PublicKeys(testIface)
	if len(got) != 2 {
		t.Fatalf("agent should hold exactly the desired peers, got %v", got)
	}
	for _, k := range got {
		if k == stale {
			t.Fatal("stale peer survived SyncPeers")
		}
	}

	if servers.counts[serverID] != 2 {
		t.Fatalf("current_peers not updated: %d", servers.counts[serverID])
	}
	if len(peers.handshakes[serverID]) != 2 {
		t.Fatalf("handshakes not recorded: %+v", peers.handshakes[serverID])
	}
}

func TestStatusReportsHealthAndInterface(t *testing.T) {
	addr, _ := startAgent(t)
	serverID := uuid.New()

	servers := &fakeServers{
		targets: map[uuid.UUID]Target{serverID: {ID: serverID, Endpoint: addr, IsActive: true}},
		counts:  map[uuid.UUID]int{},
	}
	orch := New(NewPool(newTestDialer(t, testToken)), servers, &fakePeers{})
	defer orch.Close()

	st, err := orch.Status(context.Background(), serverID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Health == nil || !st.Health.InterfaceUp || st.Health.Backend != agent.BackendMemory {
		t.Fatalf("unexpected health: %+v", st.Health)
	}
	if st.Interface == nil || st.Interface.Name != testIface {
		t.Fatalf("unexpected interface: %+v", st.Interface)
	}
}

func TestServerWithoutEndpoint(t *testing.T) {
	serverID := uuid.New()
	servers := &fakeServers{
		targets: map[uuid.UUID]Target{serverID: {ID: serverID, Endpoint: "", IsActive: true}},
		counts:  map[uuid.UUID]int{},
	}
	orch := New(NewPool(newTestDialer(t, testToken)), servers, &fakePeers{})
	defer orch.Close()

	err := orch.AddPeer(context.Background(), serverID, wireguard.NewDevicePeer(mustKey(t), "10.8.0.2"))
	if !errors.Is(err, ErrNoEndpoint) {
		t.Fatalf("expected ErrNoEndpoint, got %v", err)
	}
}

func TestUnreachableAgentIsReportedAsUnavailable(t *testing.T) {
	serverID := uuid.New()
	// Nothing listens on this port.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	servers := &fakeServers{
		targets: map[uuid.UUID]Target{serverID: {ID: serverID, Endpoint: addr, IsActive: true}},
		counts:  map[uuid.UUID]int{},
	}
	d, err := NewDialer(config.AgentConfig{Token: testToken, CallTimeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	orch := New(NewPool(d), servers, &fakePeers{})
	defer orch.Close()

	err = orch.AddPeer(context.Background(), serverID, wireguard.NewDevicePeer(mustKey(t), "10.8.0.2"))
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("expected ErrAgentUnavailable, got %v", err)
	}
}
