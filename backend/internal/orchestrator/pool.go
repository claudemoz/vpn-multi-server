package orchestrator

import (
	"errors"
	"sync"
)

// Pool keeps one AgentClient per endpoint. Connections are created on first
// use and reused afterwards; gRPC handles reconnection internally.
type Pool struct {
	dialer *Dialer

	mu      sync.Mutex
	clients map[string]*AgentClient
}

func NewPool(dialer *Dialer) *Pool {
	return &Pool{dialer: dialer, clients: make(map[string]*AgentClient)}
}

// Get returns the client for endpoint, dialing it if needed.
func (p *Pool) Get(endpoint string) (*AgentClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if c, ok := p.clients[endpoint]; ok {
		return c, nil
	}

	c, err := p.dialer.Dial(endpoint)
	if err != nil {
		return nil, err
	}
	p.clients[endpoint] = c
	return c, nil
}

// Close closes every connection.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	for endpoint, c := range p.clients {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
		delete(p.clients, endpoint)
	}
	return errors.Join(errs...)
}
