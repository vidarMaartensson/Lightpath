package eventsvc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/vidarMaartensson/Lightpath/internal/events"
	"github.com/vidarMaartensson/Lightpath/internal/network"
	"github.com/vidarMaartensson/Lightpath/internal/topocache"
)

// seedTopology serves the demo network, or err.
type seedTopology struct{ err error }

func (s seedTopology) Network(context.Context) (*network.Network, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	return network.Seed(), false, nil
}

func quiet() Option { return WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))) }

func down(id, node string) events.Event {
	return events.Event{ID: id, Kind: events.SiteDown, NodeID: node, Source: "test"}
}

func restored(id, node string) events.Event {
	return events.Event{ID: id, Kind: events.SiteRestored, NodeID: node, Source: "test"}
}

// pending returns what is buffered on s without waiting.
func pending(s *Subscription) []Update {
	var out []Update
	for {
		select {
		case u, ok := <-s.C:
			if !ok {
				return out
			}
			out = append(out, u)
		default:
			return out
		}
	}
}

func TestSubscribeStartsWithCurrentState(t *testing.T) {
	ctx := context.Background()
	s := New(seedTopology{}, quiet())
	if err := s.Publish(ctx, down("e1", "acc-ali")); err != nil {
		t.Fatal(err)
	}

	sub := s.Subscribe()
	defer sub.Close()
	if err := s.Publish(ctx, down("e2", "cab-teg-1")); err != nil {
		t.Fatal(err)
	}

	got := pending(sub)
	if len(got) != 2 {
		t.Fatalf("got %d updates, want snapshot and one change", len(got))
	}
	first, second := got[0], got[1]
	if first.Cause != nil || first.Seq != 1 || !slices.Equal(first.Down, []string{"acc-ali"}) {
		t.Errorf("snapshot = %+v, want seq 1, [acc-ali], no cause", first)
	}
	if second.Cause == nil || second.Cause.ID != "e2" || second.Cause.Seq != 2 {
		t.Errorf("change cause = %+v, want e2 with seq 2", second.Cause)
	}
	if !slices.Equal(second.Down, []string{"acc-ali", "cab-teg-1"}) {
		t.Errorf("change down = %v", second.Down)
	}
}

func TestPublishOnlyBroadcastsChanges(t *testing.T) {
	ctx := context.Background()
	s := New(seedTopology{}, quiet())
	sub := s.Subscribe()
	defer sub.Close()
	pending(sub) // snapshot

	for _, e := range []events.Event{
		down("e1", "acc-ali"),
		down("e1", "acc-ali"),       // same id
		down("e2", "acc-ali"),       // already down
		restored("e3", "cab-teg-1"), // already up
		restored("e4", "acc-ali"),
	} {
		if err := s.Publish(ctx, e); err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
	}

	var causes []string
	for _, u := range pending(sub) {
		causes = append(causes, u.Cause.ID)
	}
	if !slices.Equal(causes, []string{"e1", "e4"}) {
		t.Fatalf("broadcast %v, want [e1 e4]", causes)
	}
}

func TestUnknownNodeIsRejected(t *testing.T) {
	s := New(seedTopology{}, quiet())
	sub := s.Subscribe()
	defer sub.Close()
	pending(sub)

	err := s.Publish(context.Background(), down("e1", "acc-nowhere"))
	if !errors.Is(err, events.ErrInvalidEvent) {
		t.Fatalf("want ErrInvalidEvent, got %v", err)
	}
	if st := s.State(); st.Seq != 0 || len(st.Down) != 0 {
		t.Fatalf("state changed: %+v", st)
	}
	if got := pending(sub); len(got) != 0 {
		t.Fatalf("rejected event was broadcast: %+v", got)
	}
}

func TestTopologyOutage(t *testing.T) {
	t.Run("no snapshot: events cannot be checked", func(t *testing.T) {
		s := New(seedTopology{err: topocache.ErrUnavailable}, quiet())
		err := s.Publish(context.Background(), down("e1", "acc-ali"))
		if !errors.Is(err, topocache.ErrUnavailable) {
			t.Fatalf("want ErrUnavailable, got %v", err)
		}
		if s.State().Seq != 0 {
			t.Fatal("unchecked event changed the state")
		}
	})

	t.Run("cached snapshot: events are checked against it", func(t *testing.T) {
		src := &flakySource{}
		now := time.Unix(0, 0)
		cache := topocache.New(src, time.Minute,
			topocache.WithClock(func() time.Time { return now }),
			topocache.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
		s := New(cache, quiet())

		if err := s.Publish(context.Background(), down("e1", "acc-ali")); err != nil {
			t.Fatal(err)
		}
		src.down = true
		now = now.Add(time.Hour)
		if err := s.Publish(context.Background(), down("e2", "cab-teg-1")); err != nil {
			t.Fatalf("want the last known topology to be used, got %v", err)
		}
		if err := s.Publish(context.Background(), down("e3", "acc-nowhere")); !errors.Is(err, events.ErrInvalidEvent) {
			t.Fatalf("unknown node must still be rejected, got %v", err)
		}
	})
}

type flakySource struct{ down bool }

func (f *flakySource) Network(context.Context) (*network.Network, error) {
	if f.down {
		return nil, errors.New("connection refused")
	}
	return network.Seed(), nil
}

// TestNoUpdateIsLostOnSubscribe subscribes while events are being
// published. Each subscriber must see its snapshot followed by every later
// change, with no gap and no repeat, whenever it happened to join.
func TestNoUpdateIsLostOnSubscribe(t *testing.T) {
	ctx := context.Background()
	const writers, perWriter = 4, 50
	s := New(seedTopology{}, quiet(), WithBuffer(writers*perWriter*2+1))

	cabinets := []string{"cab-vsl-1", "cab-hag-1", "cab-cen-1", "cab-ali-1"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// Each writer toggles its own cabinet, so every event changes
			// the state.
			for i := range perWriter {
				id := fmt.Sprintf("w%d-%d", w, i)
				e := down(id, cabinets[w])
				if i%2 == 1 {
					e = restored(id, cabinets[w])
				}
				if err := s.Publish(ctx, e); err != nil {
					t.Error(err)
				}
			}
		}()
	}

	// Subscribers join from their own goroutines, spread over the run, so
	// subscriptions race with publishes.
	subs := make([]*Subscription, 50)
	for i := range subs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range i {
				runtime.Gosched()
			}
			subs[i] = s.Subscribe()
		}()
	}
	close(start)
	wg.Wait()

	total := uint64(writers * perWriter)
	for i, sub := range subs {
		sub.Close()
		got := drain(sub)
		for j := 1; j < len(got); j++ {
			if got[j] != got[j-1]+1 {
				t.Fatalf("subscriber %d: seq %d followed by %d", i, got[j-1], got[j])
			}
		}
		if last := got[len(got)-1]; last != total {
			t.Fatalf("subscriber %d ended at seq %d, want %d", i, last, total)
		}
	}
}

// fakeSource emits a fixed list of events, then waits for cancellation.
type fakeSource struct{ events []events.Event }

func (f fakeSource) Run(ctx context.Context, emit func(events.Event)) error {
	for _, e := range f.events {
		emit(e)
	}
	<-ctx.Done()
	return nil
}

func TestRunSkipsRejectedEvents(t *testing.T) {
	s := New(seedTopology{}, quiet())
	sub := s.Subscribe()
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- s.Run(ctx, fakeSource{events: []events.Event{
			down("e1", "acc-nowhere"),
			down("e2", "acc-ali"),
		}})
	}()

	<-sub.C // snapshot
	u := <-sub.C
	if u.Cause.ID != "e2" {
		t.Fatalf("first change caused by %s, want e2 (e1 is for an unknown node)", u.Cause.ID)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v after cancel", err)
	}
}
