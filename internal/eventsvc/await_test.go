package eventsvc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// countingTopology fails the first failures calls.
type countingTopology struct {
	failures int
	calls    int
}

func (c *countingTopology) Network(context.Context) (*network.Network, bool, error) {
	c.calls++
	if c.calls <= c.failures {
		return nil, false, errors.New("connection refused")
	}
	return network.Seed(), false, nil
}

func TestAwaitTopologyRetries(t *testing.T) {
	topo := &countingTopology{failures: 3}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	n, err := AwaitTopology(context.Background(), topo, time.Millisecond, 4*time.Millisecond, log)
	if err != nil {
		t.Fatal(err)
	}
	if n == nil || topo.calls != 4 {
		t.Fatalf("got network %v after %d calls, want one after 4", n != nil, topo.calls)
	}
}

func TestAwaitTopologyStopsOnCancel(t *testing.T) {
	topo := &countingTopology{failures: 1 << 30}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := AwaitTopology(ctx, topo, time.Millisecond, 2*time.Millisecond, log)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want the context error, got %v", err)
	}
}
