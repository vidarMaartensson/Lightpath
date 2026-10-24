// Command impact serves outage analyses over gRPC. It reads topology from
// the topology service and caches it.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/impactsvc"
	"github.com/vidarMaartensson/Lightpath/internal/rpc"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "impact")
	if err := run(log); err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("LISTEN_ADDR", ":9002")
	topoAddr := env("TOPOLOGY_ADDR", "localhost:9001")
	ttl, err := time.ParseDuration(env("TOPOLOGY_TTL", "30s"))
	if err != nil {
		return err
	}

	conn, err := rpc.Dial(topoAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	svc := impactsvc.New(
		rpc.TopologyClient{Client: pb.NewTopologyServiceClient(conn)},
		ttl,
		impactsvc.WithLogger(log),
	)

	srv, health := rpc.NewServer(log)
	pb.RegisterImpactServiceServer(srv, &rpc.ImpactServer{Service: svc})
	health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		health.Shutdown()
		srv.GracefulStop()
	}()
	log.Info("listening", "addr", addr, "topology", topoAddr, "ttl", ttl)
	return srv.Serve(lis)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
