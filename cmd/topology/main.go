// Command topology serves the network topology over gRPC.
//
// With DATABASE_URL set it migrates and seeds Postgres on start and reads
// from it; without it, it serves the built-in demo network from memory.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/network"
	"github.com/vidarMaartensson/Lightpath/internal/rpc"
	"github.com/vidarMaartensson/Lightpath/internal/topology"
	"github.com/vidarMaartensson/Lightpath/internal/topology/postgres"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "topology")
	if err := run(log); err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("LISTEN_ADDR", ":9001")

	var store topology.Store
	if url := os.Getenv("DATABASE_URL"); url != "" {
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			return err
		}
		defer pool.Close()
		pg := postgres.New(pool)
		if err := pg.Migrate(ctx); err != nil {
			return err
		}
		seeded, err := pg.SeedIfEmpty(ctx, network.Seed())
		if err != nil {
			return err
		}
		log.Info("postgres ready", "seeded", seeded)
		store = pg
	} else {
		mem, err := topology.NewMemoryStore(network.Seed())
		if err != nil {
			return err
		}
		log.Info("DATABASE_URL not set, serving the demo network from memory")
		store = mem
	}

	srv, health := rpc.NewServer(log)
	pb.RegisterTopologyServiceServer(srv, &rpc.TopologyServer{Store: store})
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
	log.Info("listening", "addr", addr)
	return srv.Serve(lis)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
