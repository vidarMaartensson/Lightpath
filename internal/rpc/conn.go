package rpc

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// serviceConfig gives every unary call a deadline and retries UNAVAILABLE
// with backoff. They are all reads, so retrying them is safe.
//
// EventService/WatchState is left out on purpose: it is a stream that stays
// open for as long as someone watches, and a 3s deadline would cut it off.
const serviceConfig = `{
  "methodConfig": [{
    "name": [
      {"service": "lightpath.v1.TopologyService"},
      {"service": "lightpath.v1.ImpactService"},
      {"service": "lightpath.v1.EventService", "method": "GetState"}
    ],
    "timeout": "3s",
    "retryPolicy": {
      "maxAttempts": 4,
      "initialBackoff": "0.1s",
      "maxBackoff": "1s",
      "backoffMultiplier": 2,
      "retryableStatusCodes": ["UNAVAILABLE"]
    }
  }]
}`

// Dial opens a client connection with the shared retry policy. Traffic
// between services stays on the private network, so it is plaintext here;
// in production this would be mTLS.
func Dial(addr string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(serviceConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn, nil
}

// NewServer returns a gRPC server with logging, the standard health
// service and reflection (so grpcurl works against it).
func NewServer(log *slog.Logger) (*grpc.Server, *health.Server) {
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(LoggingInterceptor(log)),
		grpc.StreamInterceptor(LoggingStreamInterceptor(log)),
	)
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	reflection.Register(srv)
	return srv, hs
}

// TopologyClient adapts the gRPC topology client to impactsvc.TopologySource.
type TopologyClient struct {
	Client pb.TopologyServiceClient
}

// Network fetches and validates the current snapshot.
func (c TopologyClient) Network(ctx context.Context) (*network.Network, error) {
	resp, err := c.Client.GetNetwork(ctx, &pb.GetNetworkRequest{})
	if err != nil {
		return nil, err
	}
	return NetworkFromProto(resp.GetNetwork())
}
