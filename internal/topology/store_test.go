package topology

import (
	"context"
	"errors"
	"testing"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

func TestMemoryStore(t *testing.T) {
	s, err := NewMemoryStore(network.Seed())
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.Network(context.Background())
	if err != nil || len(n.Nodes) == 0 {
		t.Fatalf("Network() = %v nodes, %v", len(n.Nodes), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Network(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: want context.Canceled, got %v", err)
	}
}

func TestMemoryStoreRejectsInvalidNetwork(t *testing.T) {
	if _, err := NewMemoryStore(&network.Network{}); !errors.Is(err, network.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}
