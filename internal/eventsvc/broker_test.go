package eventsvc

import (
	"slices"
	"testing"
)

func drain(s *Subscription) []uint64 {
	var seqs []uint64
	for u := range s.C {
		seqs = append(seqs, u.Seq)
	}
	return seqs
}

func TestSubscribersGetTheSameUpdatesInOrder(t *testing.T) {
	b := NewBroker(8)
	a, c := b.Subscribe(nil), b.Subscribe(nil)
	for seq := uint64(1); seq <= 5; seq++ {
		b.Publish(Update{Seq: seq})
	}
	a.Close()
	c.Close()

	want := []uint64{1, 2, 3, 4, 5}
	if got := drain(a); !slices.Equal(got, want) {
		t.Errorf("first subscriber got %v, want %v", got, want)
	}
	if got := drain(c); !slices.Equal(got, want) {
		t.Errorf("second subscriber got %v, want %v", got, want)
	}
}

func TestSlowSubscriberIsDroppedWithoutBlockingOthers(t *testing.T) {
	b := NewBroker(2)
	slow, fast := b.Subscribe(nil), b.Subscribe(nil)

	var fastGot []uint64
	for seq := uint64(1); seq <= 5; seq++ {
		b.Publish(Update{Seq: seq}) // must not block on slow
		fastGot = append(fastGot, (<-fast.C).Seq)
	}

	if got := drain(slow); !slices.Equal(got, []uint64{1, 2}) {
		t.Errorf("slow subscriber got %v, want what fit in its buffer: [1 2]", got)
	}
	if !slow.Dropped() {
		t.Error("slow subscriber should be marked dropped")
	}
	if !slices.Equal(fastGot, []uint64{1, 2, 3, 4, 5}) {
		t.Errorf("fast subscriber got %v", fastGot)
	}
	if fast.Dropped() {
		t.Error("fast subscriber was dropped")
	}
	if n := b.Subscribers(); n != 1 {
		t.Errorf("%d subscribers left, want 1", n)
	}
}

func TestCloseIsSafe(t *testing.T) {
	b := NewBroker(1)
	s := b.Subscribe(&Update{Seq: 7})
	s.Close()
	s.Close()
	b.Publish(Update{Seq: 8}) // no panic on a closed subscriber

	if got := drain(s); !slices.Equal(got, []uint64{7}) {
		t.Errorf("got %v, want the first update only", got)
	}
	if s.Dropped() {
		t.Error("a subscriber that closed itself was not dropped")
	}
	if b.Subscribers() != 0 {
		t.Error("closed subscriber still registered")
	}

	// Closing after the broker dropped it is also fine.
	d := b.Subscribe(nil)
	b.Publish(Update{Seq: 1})
	b.Publish(Update{Seq: 2})
	d.Close()
	if !d.Dropped() {
		t.Error("want dropped")
	}
}
