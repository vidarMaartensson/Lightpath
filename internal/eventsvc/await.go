package eventsvc

import (
	"context"
	"log/slog"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// AwaitTopology returns the network once topo can provide it, retrying
// with backoff from minWait up to maxWait. A source like the simulator
// needs the node list before it can start, and in docker compose the
// topology service may come up after this one. Gives up only when ctx is
// cancelled.
func AwaitTopology(ctx context.Context, topo Topology, minWait, maxWait time.Duration, log *slog.Logger) (*network.Network, error) {
	wait := minWait
	for {
		n, _, err := topo.Network(ctx)
		if err == nil {
			return n, nil
		}
		log.Warn("topology not available yet, retrying", "err", err, "in", wait.String())
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		wait = min(wait*2, maxWait)
	}
}
