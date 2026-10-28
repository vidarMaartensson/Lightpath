// Package rpc adapts the domain packages to gRPC: conversions, servers,
// clients and shared server setup.
package rpc

import (
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/events"
	"github.com/vidarMaartensson/Lightpath/internal/eventsvc"
	"github.com/vidarMaartensson/Lightpath/internal/impact"
	"github.com/vidarMaartensson/Lightpath/internal/impactsvc"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

var kindToProto = map[network.NodeKind]pb.NodeKind{
	network.KindCore:        pb.NodeKind_NODE_KIND_CORE,
	network.KindAggregation: pb.NodeKind_NODE_KIND_AGGREGATION,
	network.KindAccess:      pb.NodeKind_NODE_KIND_ACCESS,
	network.KindCabinet:     pb.NodeKind_NODE_KIND_CABINET,
}

var kindFromProto = func() map[pb.NodeKind]network.NodeKind {
	m := make(map[pb.NodeKind]network.NodeKind, len(kindToProto))
	for k, v := range kindToProto {
		m[v] = k
	}
	return m
}()

var stateToProto = map[impact.NodeState]pb.NodeState{
	impact.Online:   pb.NodeState_NODE_STATE_ONLINE,
	impact.Rerouted: pb.NodeState_NODE_STATE_REROUTED,
	impact.Isolated: pb.NodeState_NODE_STATE_ISOLATED,
	impact.Failed:   pb.NodeState_NODE_STATE_FAILED,
}

// NetworkToProto converts a domain network to its wire form.
func NetworkToProto(n *network.Network) *pb.Network {
	out := &pb.Network{
		Nodes:     make([]*pb.Node, 0, len(n.Nodes)),
		Links:     make([]*pb.Link, 0, len(n.Links)),
		Isps:      make([]*pb.Isp, 0, len(n.ISPs)),
		Customers: make([]*pb.Customer, 0, len(n.Customers)),
	}
	for _, nd := range n.Nodes {
		out.Nodes = append(out.Nodes, &pb.Node{
			Id: nd.ID, Name: nd.Name, Area: nd.Area, Kind: kindToProto[nd.Kind], X: nd.X, Y: nd.Y,
		})
	}
	for _, l := range n.Links {
		out.Links = append(out.Links, &pb.Link{Id: l.ID, NodeAId: l.A, NodeBId: l.B})
	}
	for _, i := range n.ISPs {
		out.Isps = append(out.Isps, &pb.Isp{Id: i.ID, Name: i.Name})
	}
	for _, c := range n.Customers {
		out.Customers = append(out.Customers, &pb.Customer{
			Id: c.ID, IspId: c.ISPID, NodeIds: c.NodeIDs, Business: c.Business,
		})
	}
	return out
}

// NetworkFromProto converts and validates a wire network. Data from another
// service is untrusted until it passes validation.
func NetworkFromProto(p *pb.Network) (*network.Network, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: empty network", network.ErrInvalid)
	}
	n := &network.Network{
		Nodes:     make([]network.Node, 0, len(p.GetNodes())),
		Links:     make([]network.Link, 0, len(p.GetLinks())),
		ISPs:      make([]network.ISP, 0, len(p.GetIsps())),
		Customers: make([]network.Customer, 0, len(p.GetCustomers())),
	}
	for _, nd := range p.GetNodes() {
		kind, ok := kindFromProto[nd.GetKind()]
		if !ok {
			return nil, fmt.Errorf("%w: node %q has kind %s", network.ErrInvalid, nd.GetId(), nd.GetKind())
		}
		n.Nodes = append(n.Nodes, network.Node{
			ID: nd.GetId(), Name: nd.GetName(), Area: nd.GetArea(), Kind: kind, X: nd.GetX(), Y: nd.GetY(),
		})
	}
	for _, l := range p.GetLinks() {
		n.Links = append(n.Links, network.Link{ID: l.GetId(), A: l.GetNodeAId(), B: l.GetNodeBId()})
	}
	for _, i := range p.GetIsps() {
		n.ISPs = append(n.ISPs, network.ISP{ID: i.GetId(), Name: i.GetName()})
	}
	for _, c := range p.GetCustomers() {
		n.Customers = append(n.Customers, network.Customer{
			ID: c.GetId(), ISPID: c.GetIspId(), NodeIDs: c.GetNodeIds(), Business: c.GetBusiness(),
		})
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return n, nil
}

// AnalysisToProto converts an analysis to the wire response.
func AnalysisToProto(a impactsvc.Analysis) *pb.AnalyzeOutageResponse {
	out := &pb.AnalyzeOutageResponse{
		FailedNodeIds:     a.FailedNodeIDs,
		TotalCustomers:    int32(a.TotalCustomers),
		AffectedCustomers: int32(a.AffectedCustomers),
		ReroutedCustomers: int32(a.ReroutedCustomers),
		Isps:              make([]*pb.IspImpact, 0, len(a.ISPs)),
		Nodes:             make([]*pb.NodeImpact, 0, len(a.Nodes)),
		Stale:             a.Stale,
	}
	if c := a.Change; c != nil {
		out.Change = &pb.CustomerChange{
			Lost: int32(c.Lost), Regained: int32(c.Regained), Rerouted: int32(c.Rerouted),
		}
	}
	for _, i := range a.ISPs {
		out.Isps = append(out.Isps, &pb.IspImpact{
			IspId: i.ISPID, Customers: int32(i.Customers), Affected: int32(i.Affected), Rerouted: int32(i.Rerouted),
		})
	}
	for _, n := range a.Nodes {
		out.Nodes = append(out.Nodes, &pb.NodeImpact{
			NodeId: n.NodeID, State: stateToProto[n.State],
			Customers: int32(n.Customers), Affected: int32(n.Affected), Rerouted: int32(n.Rerouted),
		})
	}
	return out
}

var eventKindToProto = map[events.Kind]pb.EventKind{
	events.SiteDown:     pb.EventKind_EVENT_KIND_SITE_DOWN,
	events.SiteRestored: pb.EventKind_EVENT_KIND_SITE_RESTORED,
}

var eventKindFromProto = func() map[pb.EventKind]events.Kind {
	m := make(map[pb.EventKind]events.Kind, len(eventKindToProto))
	for k, v := range eventKindToProto {
		m[v] = k
	}
	return m
}()

// EventToProto converts a domain event to its wire form.
func EventToProto(e events.Event) *pb.Event {
	return &pb.Event{
		Seq:    e.Seq,
		Id:     e.ID,
		Kind:   eventKindToProto[e.Kind],
		NodeId: e.NodeID,
		At:     timestamppb.New(e.At),
		Source: e.Source,
	}
}

// EventFromProto converts a wire event. Whether the node exists is left to
// the caller, which has the topology.
func EventFromProto(p *pb.Event) (events.Event, error) {
	if p == nil {
		return events.Event{}, fmt.Errorf("%w: empty event", events.ErrInvalidEvent)
	}
	kind, ok := eventKindFromProto[p.GetKind()]
	if !ok {
		return events.Event{}, fmt.Errorf("%w: event %q has kind %s", events.ErrInvalidEvent, p.GetId(), p.GetKind())
	}
	// A missing time would become 1970 and put the event first in every
	// feed, so it is rejected rather than defaulted.
	if p.GetAt() == nil {
		return events.Event{}, fmt.Errorf("%w: event %q has no time", events.ErrInvalidEvent, p.GetId())
	}
	if err := p.GetAt().CheckValid(); err != nil {
		return events.Event{}, fmt.Errorf("%w: event %q: %w", events.ErrInvalidEvent, p.GetId(), err)
	}
	return events.Event{
		Seq:    p.GetSeq(),
		ID:     p.GetId(),
		Kind:   kind,
		NodeID: p.GetNodeId(),
		At:     p.GetAt().AsTime(),
		Source: p.GetSource(),
	}, nil
}

// LiveStateToProto converts the state part of an update.
func LiveStateToProto(u eventsvc.Update) *pb.LiveState {
	return &pb.LiveState{Seq: u.Seq, DownNodeIds: u.Down}
}

// UpdateToProto converts an update for the WatchState stream. The cause is
// left unset for the snapshot a stream starts with.
func UpdateToProto(u eventsvc.Update) *pb.WatchStateResponse {
	out := &pb.WatchStateResponse{State: LiveStateToProto(u)}
	if u.Cause != nil {
		out.Cause = EventToProto(*u.Cause)
	}
	return out
}
