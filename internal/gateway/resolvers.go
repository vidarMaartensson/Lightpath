// Package gateway exposes the backend services to the browser as GraphQL.
// It owns no business logic: it translates queries into gRPC calls and
// gRPC errors into messages a user can act on.
package gateway

import (
	"context"
	_ "embed"
	"errors"
	"strings"

	"github.com/graph-gophers/graphql-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
)

//go:embed schema.graphql
var schemaSDL string

// TopologyClient is the subset of pb.TopologyServiceClient the gateway uses.
type TopologyClient interface {
	GetNetwork(ctx context.Context, in *pb.GetNetworkRequest, opts ...grpc.CallOption) (*pb.GetNetworkResponse, error)
}

// ImpactClient is the subset of pb.ImpactServiceClient the gateway uses.
type ImpactClient interface {
	AnalyzeOutage(ctx context.Context, in *pb.AnalyzeOutageRequest, opts ...grpc.CallOption) (*pb.AnalyzeOutageResponse, error)
}

// NewSchema parses the schema and binds it to the resolvers. The schema
// documents itself with quoted descriptions, which graphql-go only accepts
// with UseStringDescriptions.
func NewSchema(topology TopologyClient, impact ImpactClient) (*graphql.Schema, error) {
	return graphql.ParseSchema(schemaSDL, &Resolver{topology: topology, impact: impact},
		graphql.UseStringDescriptions())
}

// Resolver is the root resolver.
type Resolver struct {
	topology TopologyClient
	impact   ImpactClient
}

// Network resolves Query.network.
func (r *Resolver) Network(ctx context.Context) (*networkResolver, error) {
	resp, err := r.topology.GetNetwork(ctx, &pb.GetNetworkRequest{})
	if err != nil {
		return nil, userError("topology", err)
	}
	return &networkResolver{n: resp.GetNetwork()}, nil
}

// Outage resolves Query.outage.
func (r *Resolver) Outage(ctx context.Context, args struct{ FailedNodeIds []graphql.ID }) (*outageResolver, error) {
	ids := make([]string, len(args.FailedNodeIds))
	for i, id := range args.FailedNodeIds {
		ids[i] = string(id)
	}
	resp, err := r.impact.AnalyzeOutage(ctx, &pb.AnalyzeOutageRequest{FailedNodeIds: ids})
	if err != nil {
		return nil, userError("impact", err)
	}
	return &outageResolver{o: resp}, nil
}

// userError turns a gRPC error into a message for the UI. Internal details
// stay in the logs of the service that produced them.
func userError(service string, err error) error {
	switch status.Code(err) {
	case codes.InvalidArgument:
		return errors.New(status.Convert(err).Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return errors.New("the " + service + " service is not responding, try again shortly")
	default:
		return errors.New("the " + service + " service failed to answer")
	}
}

// enumName strips the protobuf prefix: NODE_KIND_CORE -> CORE.
func enumName(full, prefix string) string {
	return strings.TrimPrefix(full, prefix)
}

type networkResolver struct{ n *pb.Network }

func (r *networkResolver) Nodes() []*nodeResolver {
	out := make([]*nodeResolver, len(r.n.GetNodes()))
	for i, n := range r.n.GetNodes() {
		out[i] = &nodeResolver{n}
	}
	return out
}

func (r *networkResolver) Links() []*linkResolver {
	out := make([]*linkResolver, len(r.n.GetLinks()))
	for i, l := range r.n.GetLinks() {
		out[i] = &linkResolver{l}
	}
	return out
}

func (r *networkResolver) Isps() []*ispResolver {
	out := make([]*ispResolver, len(r.n.GetIsps()))
	for i, isp := range r.n.GetIsps() {
		out[i] = &ispResolver{isp}
	}
	return out
}

type nodeResolver struct{ n *pb.Node }

func (r *nodeResolver) ID() graphql.ID { return graphql.ID(r.n.GetId()) }
func (r *nodeResolver) Name() string   { return r.n.GetName() }
func (r *nodeResolver) Area() string   { return r.n.GetArea() }
func (r *nodeResolver) Kind() string   { return enumName(r.n.GetKind().String(), "NODE_KIND_") }
func (r *nodeResolver) X() float64     { return r.n.GetX() }
func (r *nodeResolver) Y() float64     { return r.n.GetY() }

type linkResolver struct{ l *pb.Link }

func (r *linkResolver) ID() graphql.ID { return graphql.ID(r.l.GetId()) }
func (r *linkResolver) A() graphql.ID  { return graphql.ID(r.l.GetNodeAId()) }
func (r *linkResolver) B() graphql.ID  { return graphql.ID(r.l.GetNodeBId()) }

type ispResolver struct{ i *pb.Isp }

func (r *ispResolver) ID() graphql.ID { return graphql.ID(r.i.GetId()) }
func (r *ispResolver) Name() string   { return r.i.GetName() }

type outageResolver struct{ o *pb.AnalyzeOutageResponse }

func (r *outageResolver) FailedNodeIds() []graphql.ID {
	out := make([]graphql.ID, len(r.o.GetFailedNodeIds()))
	for i, id := range r.o.GetFailedNodeIds() {
		out[i] = graphql.ID(id)
	}
	return out
}
func (r *outageResolver) TotalCustomers() int32    { return r.o.GetTotalCustomers() }
func (r *outageResolver) AffectedCustomers() int32 { return r.o.GetAffectedCustomers() }
func (r *outageResolver) ReroutedCustomers() int32 { return r.o.GetReroutedCustomers() }
func (r *outageResolver) Stale() bool              { return r.o.GetStale() }

func (r *outageResolver) Isps() []*ispImpactResolver {
	out := make([]*ispImpactResolver, len(r.o.GetIsps()))
	for i, x := range r.o.GetIsps() {
		out[i] = &ispImpactResolver{x}
	}
	return out
}

func (r *outageResolver) Nodes() []*nodeImpactResolver {
	out := make([]*nodeImpactResolver, len(r.o.GetNodes()))
	for i, x := range r.o.GetNodes() {
		out[i] = &nodeImpactResolver{x}
	}
	return out
}

type ispImpactResolver struct{ i *pb.IspImpact }

func (r *ispImpactResolver) IspID() graphql.ID { return graphql.ID(r.i.GetIspId()) }
func (r *ispImpactResolver) Customers() int32  { return r.i.GetCustomers() }
func (r *ispImpactResolver) Affected() int32   { return r.i.GetAffected() }
func (r *ispImpactResolver) Rerouted() int32   { return r.i.GetRerouted() }

type nodeImpactResolver struct{ n *pb.NodeImpact }

func (r *nodeImpactResolver) NodeID() graphql.ID { return graphql.ID(r.n.GetNodeId()) }
func (r *nodeImpactResolver) State() string {
	return enumName(r.n.GetState().String(), "NODE_STATE_")
}
func (r *nodeImpactResolver) Customers() int32 { return r.n.GetCustomers() }
func (r *nodeImpactResolver) Affected() int32  { return r.n.GetAffected() }
func (r *nodeImpactResolver) Rerouted() int32  { return r.n.GetRerouted() }
