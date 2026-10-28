// Package topocache keeps a validated topology snapshot for services that
// read topology from another service. It refreshes the snapshot at most
// once per ttl and, when a refresh fails, keeps serving the last good one.
package topocache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// ErrUnavailable: no snapshot could be loaded and none is cached.
var ErrUnavailable = errors.New("topology unavailable")

// Source loads the current network, typically over gRPC.
type Source interface {
	Network(ctx context.Context) (*network.Network, error)
}

// Cache holds the last good snapshot.
type Cache struct {
	src Source
	ttl time.Duration
	now func() time.Time
	log *slog.Logger

	mu        sync.Mutex
	snapshot  *network.Network
	fetchedAt time.Time
}

// Option configures a Cache.
type Option func(*Cache)

// WithClock replaces time.Now, for tests.
func WithClock(now func() time.Time) Option { return func(c *Cache) { c.now = now } }

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option { return func(c *Cache) { c.log = l } }

// New returns a Cache that refreshes from src at most once per ttl.
func New(src Source, ttl time.Duration, opts ...Option) *Cache {
	c := &Cache{src: src, ttl: ttl, now: time.Now, log: slog.Default()}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Network returns a snapshot, refreshing it when it is older than the ttl.
// If the refresh fails and a snapshot exists, the old one is served and
// marked stale: during an outage, a slightly old map beats no answer.
//
// The lock is held during the fetch on purpose, so a burst of requests
// after expiry triggers one fetch rather than one each.
func (c *Cache) Network(ctx context.Context) (n *network.Network, stale bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.snapshot != nil && c.now().Sub(c.fetchedAt) < c.ttl {
		return c.snapshot, false, nil
	}

	fresh, err := c.src.Network(ctx)
	if err == nil {
		err = fresh.Validate()
	}
	if err != nil {
		if c.snapshot != nil {
			c.log.Warn("topology refresh failed, serving last known snapshot",
				"err", err, "age", c.now().Sub(c.fetchedAt).Round(time.Second))
			return c.snapshot, true, nil
		}
		return nil, false, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	c.snapshot, c.fetchedAt = fresh, c.now()
	return fresh, false, nil
}
