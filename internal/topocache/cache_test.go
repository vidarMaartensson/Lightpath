package topocache

import (
	"context"
	"errors"
	"io"
	"log/slog"
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

func ok() (*network.Network, error)     { return network.Seed(), nil }
func down() (*network.Network, error)   { return nil, errors.New("connection refused") }
func broken() (*network.Network, error) { return &network.Network{}, nil }

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newCache(src Source, c *clock) *Cache {
	return New(src, time.Minute,
		WithClock(c.now),
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
}

func TestCachesWithinTTL(t *testing.T) {
	src := &fakeSource{responses: []func() (*network.Network, error){ok}}
	c := &clock{t: time.Unix(0, 0)}
	cache := newCache(src, c)

	for range 3 {
		if _, _, err := cache.Network(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if src.calls != 1 {
		t.Fatalf("fetched %d times within ttl, want 1", src.calls)
	}

	c.advance(2 * time.Minute)
	if _, _, err := cache.Network(context.Background()); err != nil {
		t.Fatal(err)
	}
	if src.calls != 2 {
		t.Fatalf("not refreshed after ttl: %d calls", src.calls)
	}
}

func TestServesStaleSnapshotWhenSourceFails(t *testing.T) {
	for name, failure := range map[string]func() (*network.Network, error){
		"source down":     down,
		"invalid network": broken,
	} {
		t.Run(name, func(t *testing.T) {
			src := &fakeSource{responses: []func() (*network.Network, error){ok, failure}}
			c := &clock{t: time.Unix(0, 0)}
			cache := newCache(src, c)

			first, _, err := cache.Network(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			c.advance(2 * time.Minute)

			n, stale, err := cache.Network(context.Background())
			if err != nil {
				t.Fatalf("want the old snapshot, got error %v", err)
			}
			if !stale || n != first {
				t.Fatalf("stale = %v, same snapshot = %v; want true, true", stale, n == first)
			}
		})
	}
}

func TestUnavailableWithoutSnapshot(t *testing.T) {
	src := &fakeSource{responses: []func() (*network.Network, error){down}}
	if _, _, err := newCache(src, &clock{}).Network(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestInvalidNetworkIsNotCached(t *testing.T) {
	src := &fakeSource{responses: []func() (*network.Network, error){broken, ok}}
	cache := newCache(src, &clock{})

	if _, _, err := cache.Network(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid network: want ErrUnavailable, got %v", err)
	}
	if _, _, err := cache.Network(context.Background()); err != nil {
		t.Fatalf("next call should fetch again and succeed: %v", err)
	}
}
