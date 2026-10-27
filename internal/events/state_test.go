package events

import (
	"errors"
	"slices"
	"testing"
)

func down(id, node string) Event     { return Event{ID: id, Kind: SiteDown, NodeID: node} }
func restored(id, node string) Event { return Event{ID: id, Kind: SiteRestored, NodeID: node} }

func TestApply(t *testing.T) {
	tests := []struct {
		name     string
		events   []Event
		changed  []bool
		wantDown []string
		wantSeq  uint64
	}{
		{
			// The scenario from the plan: Ålidhem access site goes down,
			// then comes back.
			name:     "down then restored",
			events:   []Event{down("e1", "acc-ali"), restored("e2", "acc-ali")},
			changed:  []bool{true, true},
			wantDown: []string{},
			wantSeq:  2,
		},
		{
			name:     "same id twice is ignored",
			events:   []Event{down("e1", "acc-ali"), down("e1", "acc-ali")},
			changed:  []bool{true, false},
			wantDown: []string{"acc-ali"},
			wantSeq:  1,
		},
		{
			// A redelivered down must not undo a restore that came after it.
			name:     "redelivered down after restore is ignored",
			events:   []Event{down("e1", "acc-ali"), restored("e2", "acc-ali"), down("e1", "acc-ali")},
			changed:  []bool{true, true, false},
			wantDown: []string{},
			wantSeq:  2,
		},
		{
			name:     "restore of a site that is up",
			events:   []Event{restored("e1", "acc-ali")},
			changed:  []bool{false},
			wantDown: []string{},
			wantSeq:  0,
		},
		{
			name:     "restore before down",
			events:   []Event{restored("e1", "acc-ali"), down("e2", "acc-ali")},
			changed:  []bool{false, true},
			wantDown: []string{"acc-ali"},
			wantSeq:  1,
		},
		{
			name:     "down on a site that is already down",
			events:   []Event{down("e1", "acc-ali"), down("e2", "acc-ali")},
			changed:  []bool{true, false},
			wantDown: []string{"acc-ali"},
			wantSeq:  1,
		},
		{
			name:     "down is sorted regardless of arrival order",
			events:   []Event{down("e1", "cab-3"), down("e2", "acc-ali"), down("e3", "agg-1")},
			changed:  []bool{true, true, true},
			wantDown: []string{"acc-ali", "agg-1", "cab-3"},
			wantSeq:  3,
		},
		{
			name:     "restore one of several",
			events:   []Event{down("e1", "cab-3"), down("e2", "acc-ali"), restored("e3", "cab-3")},
			changed:  []bool{true, true, true},
			wantDown: []string{"acc-ali"},
			wantSeq:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s State
			for i, e := range tt.events {
				changed, err := s.Apply(e)
				if err != nil {
					t.Fatalf("event %d: unexpected error: %v", i, err)
				}
				if changed != tt.changed[i] {
					t.Errorf("event %d: changed = %v, want %v", i, changed, tt.changed[i])
				}
			}
			if got := s.Down(); !slices.Equal(got, tt.wantDown) {
				t.Errorf("Down() = %v, want %v", got, tt.wantDown)
			}
			if s.Seq() != tt.wantSeq {
				t.Errorf("Seq() = %d, want %d", s.Seq(), tt.wantSeq)
			}
		})
	}
}

func TestApplyRejectsMalformedEvents(t *testing.T) {
	tests := []struct {
		name  string
		event Event
	}{
		{"missing id", Event{Kind: SiteDown, NodeID: "acc-ali"}},
		{"missing node", Event{ID: "e1", Kind: SiteDown}},
		{"unknown kind", Event{ID: "e1", Kind: "SITE_ON_FIRE", NodeID: "acc-ali"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s State
			changed, err := s.Apply(tt.event)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("want ErrInvalidEvent, got %v", err)
			}
			if changed || s.Seq() != 0 || len(s.Down()) != 0 {
				t.Fatalf("rejected event changed the state")
			}
		})
	}
}

func TestRejectedEventIsNotMarkedSeen(t *testing.T) {
	// A source that sends a bad event and then a corrected one with the
	// same id must not have the correction dropped as a duplicate.
	var s State
	if _, err := s.Apply(Event{ID: "e1", Kind: "BAD", NodeID: "acc-ali"}); err == nil {
		t.Fatal("want error for bad kind")
	}
	if changed, err := s.Apply(down("e1", "acc-ali")); err != nil || !changed {
		t.Fatalf("corrected event: changed=%v err=%v", changed, err)
	}
}

func TestDownReturnsACopy(t *testing.T) {
	var s State
	_, _ = s.Apply(down("e1", "acc-ali"))
	got := s.Down()
	got[0] = "tampered"
	if s.Down()[0] != "acc-ali" {
		t.Fatal("Down() exposes internal state")
	}
}
