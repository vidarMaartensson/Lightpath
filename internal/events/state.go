// Package events tracks which sites are down right now, built from a
// stream of down and restored events. Like package impact it is pure: no
// I/O and no clock. Whether a node id exists in the topology is checked by
// the service layer, which knows how to reach the topology.
package events

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Kind is what happened to a site.
type Kind string

const (
	SiteDown     Kind = "SITE_DOWN"
	SiteRestored Kind = "SITE_RESTORED"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	return k == SiteDown || k == SiteRestored
}

// ErrInvalidEvent is returned for events that are malformed regardless of
// the topology: no id, no node, or an unknown kind.
var ErrInvalidEvent = errors.New("invalid event")

// Event is one change reported by a source.
type Event struct {
	// Seq orders the changes this event caused. Set by State, not the source.
	Seq uint64
	// ID is unique per source event, so a redelivered event is ignored.
	ID     string
	Kind   Kind
	NodeID string
	At     time.Time
	// Source names where the event came from: "simulator", "manual", ...
	Source string
}

// State is which sites are down right now. The zero value is an empty
// state, ready to use. It is not safe for concurrent use; the service
// that owns it serialises access.
type State struct {
	down map[string]Event
	// seen grows with every event. At simulator rates that is a few hundred
	// ids a day, which is fine in memory; with persistence it would be
	// bounded by a retention window instead.
	seen map[string]bool
	seq  uint64
}

// Apply records e. It returns changed=false, and no error, for events that
// do not change the state: a repeated id, a site going down that is already
// down, or a site being restored that is already up. Sources can therefore
// redeliver and overlap without coordination.
func (s *State) Apply(e Event) (changed bool, err error) {
	switch {
	case e.ID == "":
		return false, fmt.Errorf("%w: missing id", ErrInvalidEvent)
	case e.NodeID == "":
		return false, fmt.Errorf("%w: event %q has no node", ErrInvalidEvent, e.ID)
	case !e.Kind.Valid():
		return false, fmt.Errorf("%w: event %q has unknown kind %q", ErrInvalidEvent, e.ID, e.Kind)
	}

	if s.seen[e.ID] {
		return false, nil
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
		s.down = map[string]Event{}
	}
	s.seen[e.ID] = true

	_, isDown := s.down[e.NodeID]
	switch {
	case e.Kind == SiteDown && !isDown:
		s.seq++
		e.Seq = s.seq
		s.down[e.NodeID] = e
	case e.Kind == SiteRestored && isDown:
		s.seq++
		delete(s.down, e.NodeID)
	default:
		return false, nil
	}
	return true, nil
}

// Seq is the number of changes applied so far. It identifies a state.
func (s *State) Seq() uint64 { return s.seq }

// Down returns the ids of the sites that are down, sorted, so equal states
// always look equal on the wire.
func (s *State) Down() []string {
	ids := make([]string, 0, len(s.down))
	for id := range s.down {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
