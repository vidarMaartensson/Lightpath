// Package impactsvc is the application layer of the impact service. It owns
// what the pure analysis in package impact should not: fetching topology,
// caching it, input limits, and what to do when topology is unavailable.
package impactsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/impact"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// MaxFailedNodes caps a single request. A larger outage is better handled
// by failing an aggregation site than by listing every cabinet.
const MaxFailedNodes = 64

var (
	// ErrInvalidInput: the request itself is wrong; retrying will not help.
	ErrInvalidInput = errors.New("invalid input")
	// ErrTopologyUnavailable: no snapshot could be loaded and none is cached.
	ErrTopologyUnavailable = errors.New("topology unavailable")
)

// TopologySource loads the current network, typically over gRPC.
type TopologySource interface {
	Network(ctx context.Context) (*network.Network, error)
}

// Analysis is an impact result plus how fresh the topology behind it is.
type Analysis struct {
	impact.Result
	// Stale is true when the topology service was unreachable and the last
	// known snapshot was used instead.
	Stale bool
}

// Service analyses outages against a cached topology snapshot.
type Service struct {
	src TopologySource
	ttl time.Duration
	now func() time.Time
	log *slog.Logger

	mu        sync.Mutex
	snapshot  *network.Network
	fetchedAt time.Time
}

// Option configures a Service.
type Option func(*Service)

// WithClock replaces time.Now, for tests.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option { return func(s *Service) { s.log = l } }

// New returns a Service that refreshes topology at most once per ttl.
func New(src TopologySource, ttl time.Duration, opts ...Option) *Service {
	s := &Service{src: src, ttl: ttl, now: time.Now, log: slog.Default()}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Analyze computes the impact of the failed nodes.
func (s *Service) Analyze(ctx context.Context, failedNodeIDs []string) (Analysis, error) {
	if len(failedNodeIDs) > MaxFailedNodes {
		return Analysis{}, fmt.Errorf("%w: at most %d failed nodes per request, got %d",
			ErrInvalidInput, MaxFailedNodes, len(failedNodeIDs))
	}
	for _, id := range failedNodeIDs {
		if id == "" {
			return Analysis{}, fmt.Errorf("%w: empty node id", ErrInvalidInput)
		}
	}

	net, stale, err := s.network(ctx)
	if err != nil {
		return Analysis{}, err
	}

	res, err := impact.Analyze(net, failedNodeIDs)
	if errors.Is(err, impact.ErrUnknownNode) {
		return Analysis{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err != nil {
		return Analysis{}, err
	}
	return Analysis{Result: res, Stale: stale}, nil
}

// network returns a snapshot, refreshing it when it is older than the ttl.
// If the refresh fails and a snapshot exists, the old one is served and
// marked stale: during an outage, a slightly old map beats no answer.
//
// The lock is held during the fetch on purpose, so a burst of requests
// after expiry triggers one fetch rather than one each.
func (s *Service) network(ctx context.Context) (*network.Network, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.snapshot != nil && s.now().Sub(s.fetchedAt) < s.ttl {
		return s.snapshot, false, nil
	}

	fresh, err := s.src.Network(ctx)
	if err == nil {
		err = fresh.Validate()
	}
	if err != nil {
		if s.snapshot != nil {
			s.log.Warn("topology refresh failed, serving last known snapshot",
				"err", err, "age", s.now().Sub(s.fetchedAt).Round(time.Second))
			return s.snapshot, true, nil
		}
		return nil, false, fmt.Errorf("%w: %w", ErrTopologyUnavailable, err)
	}

	s.snapshot, s.fetchedAt = fresh, s.now()
	return fresh, false, nil
}
