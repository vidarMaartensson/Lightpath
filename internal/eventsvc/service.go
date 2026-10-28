// Package eventsvc is the application layer of the events service. It owns
// what the pure state in package events should not: checking events
// against the topology, serialising access to the state, and fanning
// changes out to subscribers.
package eventsvc

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/vidarMaartensson/Lightpath/internal/events"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// DefaultBuffer is how many updates a subscriber may fall behind before it
// is dropped. Updates come minutes apart, so only a stuck client fills it.
const DefaultBuffer = 16

// Topology returns the network events are checked against; a
// *topocache.Cache in production.
type Topology interface {
	Network(ctx context.Context) (n *network.Network, stale bool, err error)
}

// Service holds the live state and the subscribers to it.
type Service struct {
	topo   Topology
	log    *slog.Logger
	broker *Broker

	// mu orders changes to the state with subscriptions to it, so a new
	// subscriber's snapshot and the updates that follow never overlap or
	// leave a gap.
	mu    sync.Mutex
	state events.State
}

// Option configures a Service.
type Option func(*Service)

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option { return func(s *Service) { s.log = l } }

// WithBuffer sets the per-subscriber buffer.
func WithBuffer(n int) Option { return func(s *Service) { s.broker = NewBroker(n) } }

// New returns a Service with an empty state.
func New(topo Topology, opts ...Option) *Service {
	s := &Service{topo: topo, log: slog.Default(), broker: NewBroker(DefaultBuffer)}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Publish checks e against the topology and applies it. Subscribers hear
// about it only if it changed the state. An event for a node that does not
// exist is rejected with events.ErrInvalidEvent and changes nothing.
func (s *Service) Publish(ctx context.Context, e events.Event) error {
	net, _, err := s.topo.Network(ctx)
	if err != nil {
		return fmt.Errorf("check event %q: %w", e.ID, err)
	}
	if !slices.ContainsFunc(net.Nodes, func(n network.Node) bool { return n.ID == e.NodeID }) {
		return fmt.Errorf("%w: event %q is for unknown node %q", events.ErrInvalidEvent, e.ID, e.NodeID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	changed, err := s.state.Apply(e)
	if err != nil || !changed {
		return err
	}
	e.Seq = s.state.Seq()
	u := s.currentLocked()
	u.Cause = &e
	s.broker.Publish(u)
	return nil
}

// State returns the current state.
func (s *Service) State() Update {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentLocked()
}

// Subscribe returns a subscription whose first update is the current
// state, followed by every change after it.
func (s *Service) Subscribe() *Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := s.currentLocked()
	return s.broker.Subscribe(&first)
}

// Run feeds events from src into the service until ctx is cancelled.
// Rejected events are logged and skipped: one bad event from a source
// should not stop the ones after it.
func (s *Service) Run(ctx context.Context, src events.Source) error {
	return src.Run(ctx, func(e events.Event) {
		if err := s.Publish(ctx, e); err != nil {
			s.log.Warn("event rejected", "id", e.ID, "node", e.NodeID, "source", e.Source, "err", err)
			return
		}
		s.log.Info("event", "id", e.ID, "kind", e.Kind, "node", e.NodeID, "source", e.Source)
	})
}

// Subscribers is how many clients are watching.
func (s *Service) Subscribers() int { return s.broker.Subscribers() }

func (s *Service) currentLocked() Update {
	return Update{Seq: s.state.Seq(), Down: s.state.Down()}
}
