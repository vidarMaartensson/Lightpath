// Package sim is an event source that makes sites fail and recover on
// their own, so the live view has something to show without real outage
// data. Clock and randomness are injected, which makes it deterministic in
// tests.
package sim

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/events"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// SourceName is set on every event the simulator emits.
const SourceName = "simulator"

// maxRepairFactor caps a repair at this many times the mean. Real repair
// times have a long tail, but a site that never comes back makes a dull
// demo and an unprovable test.
const maxRepairFactor = 4

// weights is the share of failures per kind. The kind is drawn first and
// the site second: the seed has four times as many cabinets as aggregation
// sites, so weighting per site would make almost every failure a cabinet.
// Core sites never fail; losing one is a different kind of story.
var weights = []struct {
	kind   network.NodeKind
	weight int
}{
	{network.KindCabinet, 70},
	{network.KindAccess, 25},
	{network.KindAggregation, 5},
}

// ErrInvalidConfig is returned by New when the simulator cannot run.
var ErrInvalidConfig = errors.New("invalid simulator config")

// Config sets how busy the simulated network is.
type Config struct {
	// Interval is the mean time between failures.
	Interval time.Duration
	// MeanRepair is the mean time a failed site stays down.
	MeanRepair time.Duration
	// MaxConcurrent caps how many sites are down at once.
	MaxConcurrent int
}

// DefaultConfig keeps a few sites down most of the time without turning
// the map red.
var DefaultConfig = Config{Interval: 3 * time.Minute, MeanRepair: 6 * time.Minute, MaxConcurrent: 3}

// Simulator implements events.Source. It is not safe for concurrent use;
// Run owns it.
type Simulator struct {
	cfg    Config
	byKind map[network.NodeKind][]string
	rng    *rand.Rand
	now    func() time.Time
	after  func(time.Duration) <-chan time.Time

	idPrefix    string
	seq         int
	nextFailure time.Time
	repairs     map[string]time.Time // node id -> when it comes back
}

// Option configures a Simulator.
type Option func(*Simulator)

// WithClock replaces time.Now, for tests.
func WithClock(now func() time.Time) Option { return func(s *Simulator) { s.now = now } }

// WithRand sets the random source. A fixed seed gives a fixed sequence.
func WithRand(r *rand.Rand) Option { return func(s *Simulator) { s.rng = r } }

// WithAfter replaces time.After, for tests.
func WithAfter(after func(time.Duration) <-chan time.Time) Option {
	return func(s *Simulator) { s.after = after }
}

var _ events.Source = (*Simulator)(nil)

// New returns a simulator that fails sites among nodes. The node list is
// read once; topology changes while running are not picked up.
func New(nodes []network.Node, cfg Config, opts ...Option) (*Simulator, error) {
	switch {
	case cfg.Interval <= 0:
		return nil, fmt.Errorf("%w: interval must be positive, got %s", ErrInvalidConfig, cfg.Interval)
	case cfg.MeanRepair <= 0:
		return nil, fmt.Errorf("%w: mean repair must be positive, got %s", ErrInvalidConfig, cfg.MeanRepair)
	case cfg.MaxConcurrent < 1:
		return nil, fmt.Errorf("%w: max concurrent must be at least 1, got %d", ErrInvalidConfig, cfg.MaxConcurrent)
	}

	s := &Simulator{
		cfg:     cfg,
		byKind:  map[network.NodeKind][]string{},
		now:     time.Now,
		after:   time.After,
		repairs: map[string]time.Time{},
	}
	for _, o := range opts {
		o(s)
	}
	if s.rng == nil {
		s.rng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
	}

	failable := 0
	for _, w := range weights {
		for _, n := range nodes {
			if n.Kind == w.kind {
				s.byKind[w.kind] = append(s.byKind[w.kind], n.ID)
				failable++
			}
		}
	}
	if failable == 0 {
		return nil, fmt.Errorf("%w: no sites that can fail", ErrInvalidConfig)
	}

	// The start time is part of every id, so ids from an earlier run of
	// the simulator never collide with this one when events are replayed.
	start := s.now()
	s.idPrefix = fmt.Sprintf("sim-%d", start.UnixNano())
	s.nextFailure = start.Add(s.exp(cfg.Interval))
	return s, nil
}

// Run implements events.Source.
func (s *Simulator) Run(ctx context.Context, emit func(events.Event)) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.after(s.next().Sub(s.now())):
		}
		for _, e := range s.step(s.now()) {
			emit(e)
		}
	}
}

// next is when something will happen: a failure or a repair.
func (s *Simulator) next() time.Time {
	if _, at, ok := s.nextRepair(); ok && at.Before(s.nextFailure) {
		return at
	}
	return s.nextFailure
}

// step returns every event due at or before now, in time order. Repairs
// due at the same moment as a failure go first, so they free their slot.
func (s *Simulator) step(now time.Time) []events.Event {
	var out []events.Event
	for {
		node, at, ok := s.nextRepair()
		if ok && !at.After(now) && !at.After(s.nextFailure) {
			delete(s.repairs, node)
			out = append(out, s.event(events.SiteRestored, node, at))
			continue
		}
		if s.nextFailure.After(now) {
			return out
		}
		at = s.nextFailure
		s.nextFailure = at.Add(s.exp(s.cfg.Interval))
		// At the cap the failure is skipped rather than queued: a queue
		// would release a burst of failures as soon as repairs catch up.
		if len(s.repairs) >= s.cfg.MaxConcurrent {
			continue
		}
		if node, ok := s.pick(); ok {
			repair := min(s.exp(s.cfg.MeanRepair), maxRepairFactor*s.cfg.MeanRepair)
			s.repairs[node] = at.Add(repair)
			out = append(out, s.event(events.SiteDown, node, at))
		}
	}
}

// nextRepair is the earliest scheduled repair. Ties break on node id so
// the order does not depend on map iteration.
func (s *Simulator) nextRepair() (node string, at time.Time, ok bool) {
	for id, t := range s.repairs {
		if !ok || t.Before(at) || (t.Equal(at) && id < node) {
			node, at, ok = id, t, true
		}
	}
	return node, at, ok
}

// pick draws a kind by weight among kinds that still have a site up, then
// a site of that kind uniformly.
func (s *Simulator) pick() (string, bool) {
	type option struct {
		weight int
		up     []string
	}
	var opts []option
	total := 0
	for _, w := range weights {
		var up []string
		for _, id := range s.byKind[w.kind] {
			if _, down := s.repairs[id]; !down {
				up = append(up, id)
			}
		}
		if len(up) > 0 {
			opts = append(opts, option{w.weight, up})
			total += w.weight
		}
	}
	if total == 0 {
		return "", false
	}
	r := s.rng.IntN(total)
	for _, o := range opts {
		if r < o.weight {
			return o.up[s.rng.IntN(len(o.up))], true
		}
		r -= o.weight
	}
	return "", false // not reached: r < total
}

func (s *Simulator) event(kind events.Kind, node string, at time.Time) events.Event {
	s.seq++
	return events.Event{
		ID:     fmt.Sprintf("%s-%d", s.idPrefix, s.seq),
		Kind:   kind,
		NodeID: node,
		At:     at,
		Source: SourceName,
	}
}

// exp draws from an exponential distribution with the given mean, the
// usual model for independent failures. It never returns zero, so time
// always moves forward.
func (s *Simulator) exp(mean time.Duration) time.Duration {
	return max(time.Duration(s.rng.ExpFloat64()*float64(mean)), time.Millisecond)
}
