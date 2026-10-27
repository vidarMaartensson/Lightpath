package sim

import (
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/events"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

var start = time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC)

// fixture is a simulator on the demo seed with a fake clock.
type fixture struct {
	sim   *Simulator
	clock time.Time
}

func newFixture(t *testing.T, cfg Config, seed uint64) *fixture {
	t.Helper()
	f := &fixture{clock: start}
	s, err := New(network.Seed().Nodes, cfg,
		WithClock(func() time.Time { return f.clock }),
		WithRand(rand.New(rand.NewPCG(seed, 0))),
	)
	if err != nil {
		t.Fatal(err)
	}
	f.sim = s
	return f
}

// run advances the fake clock event by event for d and returns everything
// the simulator emitted.
func (f *fixture) run(d time.Duration) []events.Event {
	end := f.clock.Add(d)
	var out []events.Event
	for {
		next := f.sim.next()
		if next.After(end) {
			f.clock = end
			return out
		}
		f.clock = next
		out = append(out, f.sim.step(next)...)
	}
}

func kinds() map[string]network.NodeKind {
	m := map[string]network.NodeKind{}
	for _, n := range network.Seed().Nodes {
		m[n.ID] = n.Kind
	}
	return m
}

func TestStateStaysWithinLimits(t *testing.T) {
	f := newFixture(t, DefaultConfig, 1)
	evs := f.run(7 * 24 * time.Hour)
	if len(evs) < 1000 {
		t.Fatalf("a simulated week should be busy, got %d events", len(evs))
	}

	// Every event must be accepted by State, which ties the simulator to
	// the domain model it feeds.
	var st events.State
	kindOf := kinds()
	for i, e := range evs {
		if e.Source != SourceName {
			t.Fatalf("event %d: source %q", i, e.Source)
		}
		if kindOf[e.NodeID] == network.KindCore {
			t.Fatalf("event %d: core site %s failed", i, e.NodeID)
		}
		if i > 0 && e.At.Before(evs[i-1].At) {
			t.Fatalf("event %d: out of order (%s before %s)", i, e.At, evs[i-1].At)
		}
		changed, err := st.Apply(e)
		if err != nil {
			t.Fatalf("event %d rejected: %v", i, err)
		}
		if !changed {
			t.Fatalf("event %d changed nothing: %+v", i, e)
		}
		if n := len(st.Down()); n > DefaultConfig.MaxConcurrent {
			t.Fatalf("event %d: %d sites down, max is %d", i, n, DefaultConfig.MaxConcurrent)
		}
	}
}

func TestEverySiteIsRepaired(t *testing.T) {
	f := newFixture(t, DefaultConfig, 2)
	evs := f.run(7 * 24 * time.Hour)
	limit := maxRepairFactor * DefaultConfig.MeanRepair

	downAt := map[string]time.Time{}
	for _, e := range evs {
		switch e.Kind {
		case events.SiteDown:
			downAt[e.NodeID] = e.At
		case events.SiteRestored:
			if took := e.At.Sub(downAt[e.NodeID]); took > limit {
				t.Errorf("%s took %s to repair, limit is %s", e.NodeID, took, limit)
			}
			delete(downAt, e.NodeID)
		}
	}
	// Sites still down at the end must be recent, not forgotten.
	for id, at := range downAt {
		if age := f.clock.Sub(at); age > limit {
			t.Errorf("%s down for %s and never repaired", id, age)
		}
	}
}

func TestSameSeedSameSequence(t *testing.T) {
	a := newFixture(t, DefaultConfig, 42).run(24 * time.Hour)
	b := newFixture(t, DefaultConfig, 42).run(24 * time.Hour)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed and clock gave different events")
	}
	c := newFixture(t, DefaultConfig, 43).run(24 * time.Hour)
	if reflect.DeepEqual(a, c) {
		t.Fatal("different seeds gave the same events")
	}
}

func TestFailuresFollowWeights(t *testing.T) {
	// A high cap and quick repairs, so skipped failures and kinds that are
	// fully down do not skew the shares.
	cfg := Config{Interval: 3 * time.Minute, MeanRepair: 30 * time.Second, MaxConcurrent: 30}
	evs := newFixture(t, cfg, 3).run(60 * 24 * time.Hour)

	kindOf := kinds()
	count := map[network.NodeKind]int{}
	total := 0
	for _, e := range evs {
		if e.Kind == events.SiteDown {
			count[kindOf[e.NodeID]]++
			total++
		}
	}
	for _, w := range weights {
		got := 100 * float64(count[w.kind]) / float64(total)
		if diff := got - float64(w.weight); diff < -2 || diff > 2 {
			t.Errorf("%s: %.1f%% of failures, want about %d%%", w.kind, got, w.weight)
		}
	}
}

func TestRunEmitsUntilCancelled(t *testing.T) {
	clock := start
	s, err := New(network.Seed().Nodes, DefaultConfig,
		WithClock(func() time.Time { return clock }),
		WithRand(rand.New(rand.NewPCG(4, 0))),
		// Fires at once and moves the clock to the requested time, so Run
		// works through simulated hours without sleeping.
		WithAfter(func(d time.Duration) <-chan time.Time {
			clock = clock.Add(d)
			ch := make(chan time.Time, 1)
			ch <- clock
			return ch
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var got []events.Event
	err = s.Run(ctx, func(e events.Event) {
		got = append(got, e)
		if len(got) == 10 {
			cancel()
		}
	})
	if err != nil {
		t.Fatalf("Run returned %v after cancel, want nil", err)
	}
	if len(got) < 10 {
		t.Fatalf("got %d events before cancel, want at least 10", len(got))
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	nodes := network.Seed().Nodes
	tests := []struct {
		name  string
		cfg   Config
		nodes []network.Node
	}{
		{"zero interval", Config{Interval: 0, MeanRepair: time.Minute, MaxConcurrent: 1}, nodes},
		{"negative repair", Config{Interval: time.Minute, MeanRepair: -time.Minute, MaxConcurrent: 1}, nodes},
		{"no concurrency", Config{Interval: time.Minute, MeanRepair: time.Minute, MaxConcurrent: 0}, nodes},
		{"only core sites", DefaultConfig, []network.Node{{ID: "core-a", Kind: network.KindCore}}},
		{"no nodes", DefaultConfig, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.nodes, tt.cfg); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("want ErrInvalidConfig, got %v", err)
			}
		})
	}
}
