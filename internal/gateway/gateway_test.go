package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/impactsvc"
	"github.com/vidarMaartensson/Lightpath/internal/network"
	"github.com/vidarMaartensson/Lightpath/internal/rpc"
)

// fakeTopology serves the seed network without a real gRPC hop.
type fakeTopology struct{ err error }

func (f fakeTopology) GetNetwork(context.Context, *pb.GetNetworkRequest, ...grpc.CallOption) (*pb.GetNetworkResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pb.GetNetworkResponse{Network: rpc.NetworkToProto(network.Seed())}, nil
}

// fakeImpact runs the real impact service in-process.
type fakeImpact struct{ svc *impactsvc.Service }

func (f fakeImpact) AnalyzeOutage(ctx context.Context, in *pb.AnalyzeOutageRequest, _ ...grpc.CallOption) (*pb.AnalyzeOutageResponse, error) {
	return (&rpc.ImpactServer{Service: f.svc}).AnalyzeOutage(ctx, in)
}

type memSource struct{}

func (memSource) Network(context.Context) (*network.Network, error) { return network.Seed(), nil }

func newServer(t *testing.T, topo TopologyClient) *httptest.Server {
	t.Helper()
	schema, err := NewSchema(topo, fakeImpact{svc: impactsvc.New(memSource{}, 0)}, nil)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(Handler(schema, nil, "", log))
	t.Cleanup(srv.Close)
	return srv
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func post(t *testing.T, url, query string, vars map[string]any) gqlResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	resp, err := http.Post(url+"/graphql", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out gqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNetworkQuery(t *testing.T) {
	srv := newServer(t, fakeTopology{})
	res := post(t, srv.URL, `{ network { nodes { id kind x y } links { a b } isps { name } } }`, nil)
	if len(res.Errors) > 0 {
		t.Fatalf("errors: %+v", res.Errors)
	}
	var data struct {
		Network struct {
			Nodes []struct{ ID, Kind string }
		}
	}
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Network.Nodes) == 0 || data.Network.Nodes[0].Kind != "CORE" {
		t.Fatalf("unexpected nodes: %+v", data.Network.Nodes)
	}
}

func TestOutageQuery(t *testing.T) {
	srv := newServer(t, fakeTopology{})
	q := `query($ids: [ID!]!) { outage(failedNodeIds: $ids) {
		affectedCustomers reroutedCustomers stale
		isps { ispId affected }
		nodes { nodeId state }
	} }`
	res := post(t, srv.URL, q, map[string]any{"ids": []string{"acc-ali"}})
	if len(res.Errors) > 0 {
		t.Fatalf("errors: %+v", res.Errors)
	}
	var data struct {
		Outage struct {
			AffectedCustomers, ReroutedCustomers int
			Nodes                                []struct{ NodeID, State string }
		}
	}
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Outage.AffectedCustomers == 0 || data.Outage.ReroutedCustomers == 0 {
		t.Fatalf("want affected and rerouted customers, got %+v", data.Outage)
	}
	for _, n := range data.Outage.Nodes {
		if n.NodeID == "acc-ali" && n.State != "FAILED" {
			t.Fatalf("acc-ali state = %s, want FAILED", n.State)
		}
	}
}

func TestOutageUnknownNodeIsAUserError(t *testing.T) {
	srv := newServer(t, fakeTopology{})
	res := post(t, srv.URL, `{ outage(failedNodeIds: ["acc-nowhere"]) { affectedCustomers } }`, nil)
	if len(res.Errors) == 0 || !strings.Contains(res.Errors[0].Message, "unknown node") {
		t.Fatalf("want an unknown node error, got %+v", res.Errors)
	}
}

func TestTopologyDownIsReportedPlainly(t *testing.T) {
	srv := newServer(t, fakeTopology{err: status.Error(codes.Unavailable, "dial tcp: connection refused")})
	res := post(t, srv.URL, `{ network { nodes { id } } }`, nil)
	if len(res.Errors) == 0 {
		t.Fatal("want an error")
	}
	msg := res.Errors[0].Message
	if !strings.Contains(msg, "not responding") || strings.Contains(msg, "dial tcp") {
		t.Fatalf("error should be plain and not leak internals: %q", msg)
	}
}

func TestOutageChange(t *testing.T) {
	srv := newServer(t, fakeTopology{})
	q := `query($ids: [ID!]!, $base: [ID!]) { outage(failedNodeIds: $ids, baselineNodeIds: $base) {
		change { lost regained rerouted }
	} }`
	type change struct{ Lost, Regained, Rerouted int }
	tests := []struct {
		name string
		vars map[string]any
		want *change
	}{
		{"no baseline gives no change", map[string]any{"ids": []string{"acc-ali"}}, nil},
		{"empty baseline means nothing was down", map[string]any{"ids": []string{"acc-ali"}, "base": []string{}},
			&change{Lost: 76, Rerouted: 104}},
		{"restore", map[string]any{"ids": []string{}, "base": []string{"acc-ali"}}, &change{Regained: 76}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := post(t, srv.URL, q, tt.vars)
			if len(res.Errors) > 0 {
				t.Fatalf("errors: %+v", res.Errors)
			}
			var data struct{ Outage struct{ Change *change } }
			if err := json.Unmarshal(res.Data, &data); err != nil {
				t.Fatal(err)
			}
			got := data.Outage.Change
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Fatalf("change = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOutageUnknownBaselineNodeIsAUserError(t *testing.T) {
	srv := newServer(t, fakeTopology{})
	res := post(t, srv.URL, `{ outage(failedNodeIds: [], baselineNodeIds: ["acc-nowhere"]) { affectedCustomers } }`, nil)
	if len(res.Errors) == 0 || !strings.Contains(res.Errors[0].Message, "baseline") {
		t.Fatalf("want an error naming the baseline, got %+v", res.Errors)
	}
}
