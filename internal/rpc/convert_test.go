package rpc

import (
	"errors"
	"reflect"
	"testing"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
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
