package eventsvc

import (
	"sync"

	"github.com/vidarMaartensson/Lightpath/internal/events"
)

// Update is one state of the live view as subscribers see it.
type Update struct {
	Seq  uint64
	Down []string
	// Cause is the event behind this change. Nil for the snapshot a
	// subscriber gets first, which describes the state, not an event.
	Cause *events.Event
}

// Broker fans updates out to subscribers. Each subscriber has its own
// buffer; one that falls behind is dropped rather than allowed to block
// the others. A dropped client reconnects and gets the full state again,
// so nothing is lost by dropping it.
type Broker struct {
	buffer int

	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

// Subscription receives updates on C until it is closed, by the
// subscriber or by the broker because the subscriber was too slow.
type Subscription struct {
	C <-chan Update

	ch      chan Update
	broker  *Broker
	dropped bool // guarded by broker.mu
	closed  bool // guarded by broker.mu
}

// NewBroker returns a broker that buffers up to buffer updates per
// subscriber.
func NewBroker(buffer int) *Broker {
	return &Broker{buffer: max(buffer, 1), subs: map[*Subscription]struct{}{}}
}

// Subscribe registers a subscriber. If first is not nil it is the first
// update the subscriber receives.
func (b *Broker) Subscribe(first *Update) *Subscription {
	ch := make(chan Update, b.buffer)
	if first != nil {
		ch <- *first
	}
	s := &Subscription{C: ch, ch: ch, broker: b}

	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

// Publish sends u to every subscriber without blocking.
func (b *Broker) Publish(u Update) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		select {
		case s.ch <- u:
		default:
			s.dropped = true
			b.removeLocked(s)
		}
	}
}

// Subscribers is how many subscribers are registered.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

func (b *Broker) removeLocked(s *Subscription) {
	if s.closed {
		return
	}
	s.closed = true
	delete(b.subs, s)
	close(s.ch)
}

// Close unsubscribes. It is safe to call more than once and after the
// broker has dropped the subscription.
func (s *Subscription) Close() {
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	s.broker.removeLocked(s)
}

// Dropped reports whether the broker closed the subscription because the
// subscriber did not keep up. Meaningful once C is closed.
func (s *Subscription) Dropped() bool {
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	return s.dropped
}
