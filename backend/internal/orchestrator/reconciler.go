package orchestrator

import (
	"context"
	"log"
	"sync"
	"time"
)

// Reconciler periodically resyncs every active server with the database. It
// is the safety net that makes the platform converge after an agent restart,
// a missed push (agent down during a device creation) or a manual change on
// the server, and it is what feeds device.last_handshake.
type Reconciler struct {
	orch     *Orchestrator
	interval time.Duration
	logger   *log.Logger
}

func NewReconciler(orch *Orchestrator, interval time.Duration, logger *log.Logger) *Reconciler {
	if logger == nil {
		logger = log.Default()
	}
	return &Reconciler{orch: orch, interval: interval, logger: logger}
}

// Run blocks until ctx is cancelled. A first pass runs immediately so agents
// are consistent right after the API starts.
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Printf("reconciler started (interval=%s)", r.interval)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			r.logger.Println("reconciler stopped")
			return
		case <-ticker.C:
			r.RunOnce(ctx)
		}
	}
}

// RunOnce syncs every active server concurrently. Failures are logged per
// server and never abort the others.
func (r *Reconciler) RunOnce(ctx context.Context) {
	targets, err := r.orch.servers.ActiveTargets()
	if err != nil {
		r.logger.Printf("reconciler: list servers: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, t := range targets {
		if t.Endpoint == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := r.orch.SyncServer(ctx, t.ID)
			if err != nil {
				r.logger.Printf("reconciler: %s (%s): %v", t.Name, t.Endpoint, err)
				return
			}
			r.logger.Printf("reconciler: %s (%s): %d peers, %d online in %s",
				t.Name, t.Endpoint, res.PeerCount, res.Online, res.Duration)
		}()
	}
	wg.Wait()
}
