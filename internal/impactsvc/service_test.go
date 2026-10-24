package impactsvc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// fakeSource returns queued responses in order and counts calls.
type fakeSource struct {
	calls     int
	responses []func() (*network.Network, error)
}

func (f *fakeSource) Network(context.Context) (*network.Network, error) {
	f.calls++
	r := f.responses[0]
	if len(f.responses) > 1 {
		f.responses = f.responses[1:]
	}
	return r()
}

func ok() (*network.Network, error) { return network.Seed(), nil }
func down() (*network.Network, error) {
	return nil, errors.New("connection refused")
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newService(src TopologySource, c *clock) *Service {
	return New(src, time.Minute,
		WithClock(c.now),
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
}

func TestCachesWithinTTL(t *testing.T) {
	src := &fakeSource{responses: []func() (*network.Network, error){ok}}
	c := &clock{t: time.Unix(0, 0)}
	s := newService(src, c)

	for range 3 {
		if _, err := s.Analyze(context.Background(), []string{"acc-ali"}); err != nil {
			t.Fatal(err)
		}
	}
	if src.calls != 1 {
		t.Fatalf("topology fetched %d times within ttl, want 1", src.calls)
	}

	c.advance(2 * time.Minute)
	if _, err := s.Analyze(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if src.calls != 2 {
		t.Fatalf("topology not refreshed after ttl: %d calls", src.calls)
	}
}

func TestServesStaleSnapshotWhenTopologyIsDown(t *testing.T) {
	src := &fakeSource{responses: []func() (*network.Network, error){ok, down}}
	c := &clock{t: time.Unix(0, 0)}
	s := newService(src, c)

	if _, err := s.Analyze(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	c.advance(2 * time.Minute)

	a, err := s.Analyze(context.Background(), []string{"acc-ali"})
	if err != nil {
		t.Fatalf("want stale answer, got error %v", err)
	}
	if !a.Stale {
		t.Fatal("answer from an old snapshot must be marked stale")
	}
	if a.AffectedCustomers == 0 {
		t.Fatal("stale snapshot should still produce an analysis")
	}
}

func TestUnavailableWithoutSnapshot(t *testing.T) {
	src := &fakeSource{responses: []func() (*network.Network, error){down}}
	s := newService(src, &clock{})

	_, err := s.Analyze(context.Background(), nil)
	if !errors.Is(err, ErrTopologyUnavailable) {
		t.Fatalf("want ErrTopologyUnavailable, got %v", err)
	}
}

func TestBadTopologyIsNotCached(t *testing.T) {
	broken := func() (*network.Network, error) { return &network.Network{}, nil }
	src := &fakeSource{responses: []func() (*network.Network, error){broken, ok}}
	s := newService(src, &clock{})

	if _, err := s.Analyze(context.Background(), nil); !errors.Is(err, ErrTopologyUnavailable) {
		t.Fatalf("invalid snapshot: want ErrTopologyUnavailable, got %v", err)
	}
	if _, err := s.Analyze(context.Background(), nil); err != nil {
		t.Fatalf("next request should fetch again and succeed: %v", err)
	}
}

func TestInvalidInput(t *testing.T) {
	tooMany := make([]string, MaxFailedNodes+1)
	for i := range tooMany {
		tooMany[i] = "n" + strconv.Itoa(i)
	}
	tests := map[string][]string{
		"unknown node": {"acc-ali", "acc-nowhere"},
		"empty id":     {""},
		"too many":     tooMany,
	}
	for name, ids := range tests {
		t.Run(name, func(t *testing.T) {
			src := &fakeSource{responses: []func() (*network.Network, error){ok}}
			s := newService(src, &clock{})
			if _, err := s.Analyze(context.Background(), ids); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("want ErrInvalidInput, got %v", err)
			}
		})
	}
}
