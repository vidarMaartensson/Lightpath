package rpc

import (
	"errors"
	"reflect"
	"testing"
	"time"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/events"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

func TestNetworkRoundTrip(t *testing.T) {
	in := network.Seed()
	out, err := NetworkFromProto(NetworkToProto(in))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatal("network changed after a round trip through protobuf")
	}
}

func TestNetworkFromProtoRejectsBadData(t *testing.T) {
	p := NetworkToProto(network.Seed())
	p.Nodes[0].Kind = pb.NodeKind_NODE_KIND_UNSPECIFIED
	if _, err := NetworkFromProto(p); !errors.Is(err, network.ErrInvalid) {
		t.Fatalf("unspecified kind: want ErrInvalid, got %v", err)
	}

	if _, err := NetworkFromProto(nil); !errors.Is(err, network.ErrInvalid) {
		t.Fatalf("nil network: want ErrInvalid, got %v", err)
	}
}

func TestEventRoundTrip(t *testing.T) {
	for _, kind := range []events.Kind{events.SiteDown, events.SiteRestored} {
		in := events.Event{
			Seq:    42,
			ID:     "sim-1-7",
			Kind:   kind,
			NodeID: "acc-ali",
			At:     time.Date(2026, 10, 24, 14, 2, 0, 0, time.UTC),
			Source: "simulator",
		}
		out, err := EventFromProto(EventToProto(in))
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if !reflect.DeepEqual(in, out) {
			t.Fatalf("%s: event changed after a round trip: %+v", kind, out)
		}
	}
}

func TestEventFromProtoRejectsBadData(t *testing.T) {
	valid := func() *pb.Event {
		return EventToProto(events.Event{
			ID: "e1", Kind: events.SiteDown, NodeID: "acc-ali", At: time.Unix(1, 0),
		})
	}
	tests := []struct {
		name string
		p    *pb.Event
	}{
		{"nil event", nil},
		{"unspecified kind", func() *pb.Event { p := valid(); p.Kind = pb.EventKind_EVENT_KIND_UNSPECIFIED; return p }()},
		{"kind from a newer client", func() *pb.Event { p := valid(); p.Kind = 99; return p }()},
		{"missing time", func() *pb.Event { p := valid(); p.At = nil; return p }()},
		{"time out of range", func() *pb.Event { p := valid(); p.At.Nanos = -1; return p }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := EventFromProto(tt.p); !errors.Is(err, events.ErrInvalidEvent) {
				t.Fatalf("want ErrInvalidEvent, got %v", err)
			}
		})
	}
}
