// Package topology serves network snapshots from a store.
package topology

import (
	"context"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// Store loads the current network.
type Store interface {
	Network(ctx context.Context) (*network.Network, error)
}

// MemoryStore serves a fixed snapshot. Used for local development without
// Postgres and in tests.
type MemoryStore struct {
	net *network.Network
}

// NewMemoryStore validates n and returns a store serving it.
func NewMemoryStore(n *network.Network) (*MemoryStore, error) {
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return &MemoryStore{net: n}, nil
}

// Network returns the snapshot.
func (s *MemoryStore) Network(ctx context.Context) (*network.Network, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.net, nil
}
