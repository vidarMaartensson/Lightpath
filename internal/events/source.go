package events

import "context"

// Source produces events: the simulator now, a scraper of real outage
// notices later. The service treats every source the same way.
type Source interface {
	// Run sends events to emit until ctx is cancelled. Cancellation is a
	// normal stop and returns nil; any other return means the source failed.
	Run(ctx context.Context, emit func(Event)) error
}
