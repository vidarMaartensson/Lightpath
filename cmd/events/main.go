// Command events serves which sites are down right now over gRPC.
//
// With LIGHTPATH_MODE=live it runs the simulator, so sites fail and recover
// on their own. With demo (the default) it serves an empty state and
// nothing happens unless a source is added. SIM_INTERVAL, SIM_MEAN_REPAIR
// and SIM_MAX_CONCURRENT tune the simulator; the browser tests use them to
// see events within seconds.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/events/sim"
	"github.com/vidarMaartensson/Lightpath/internal/eventsvc"
	"github.com/vidarMaartensson/Lightpath/internal/rpc"
	"github.com/vidarMaartensson/Lightpath/internal/topocache"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "events")
	if err := run(log); err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("LISTEN_ADDR", ":9003")
	topoAddr := env("TOPOLOGY_ADDR", "localhost:9001")
	mode := env("LIGHTPATH_MODE", "demo")
	if mode != "live" && mode != "demo" {
		return fmt.Errorf("LIGHTPATH_MODE must be live or demo, got %q", mode)
	}
	ttl, err := time.ParseDuration(env("TOPOLOGY_TTL", "30s"))
	if err != nil {
		return err
	}
	simCfg, err := simConfig()
	if err != nil {
		return err
	}

	conn, err := rpc.Dial(topoAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	topo := topocache.New(rpc.TopologyClient{Client: pb.NewTopologyServiceClient(conn)}, ttl,
		topocache.WithLogger(log))
	svc := eventsvc.New(topo, eventsvc.WithLogger(log))

	if mode == "live" {
		go simulate(ctx, svc, topo, simCfg, log)
	}

	srv, health := rpc.NewServer(log)
	pb.RegisterEventServiceServer(srv, &rpc.EventServer{Service: svc, Live: mode == "live"})
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
	log.Info("listening", "addr", addr, "topology", topoAddr, "mode", mode)
	return srv.Serve(lis)
}

// simulate waits for the topology, then feeds simulated events into svc
// until ctx is cancelled. The server answers in the meantime, with an
// empty state.
func simulate(ctx context.Context, svc *eventsvc.Service, topo *topocache.Cache, cfg sim.Config, log *slog.Logger) {
	n, err := eventsvc.AwaitTopology(ctx, topo, time.Second, 30*time.Second, log)
	if err != nil {
		return // shutting down
	}
	s, err := sim.New(n.Nodes, cfg)
	if err != nil {
		log.Error("simulator not started", "err", err)
		return
	}
	log.Info("simulator started", "interval", cfg.Interval.String(),
		"mean_repair", cfg.MeanRepair.String(), "max_concurrent", cfg.MaxConcurrent)
	if err := svc.Run(ctx, s); err != nil {
		log.Error("simulator stopped", "err", err)
	}
}

// simConfig reads the simulator settings, falling back to the defaults.
// sim.New checks that the values make sense.
func simConfig() (sim.Config, error) {
	cfg := sim.DefaultConfig
	var err error
	if v := os.Getenv("SIM_INTERVAL"); v != "" {
		if cfg.Interval, err = time.ParseDuration(v); err != nil {
			return cfg, fmt.Errorf("SIM_INTERVAL: %w", err)
		}
	}
	if v := os.Getenv("SIM_MEAN_REPAIR"); v != "" {
		if cfg.MeanRepair, err = time.ParseDuration(v); err != nil {
			return cfg, fmt.Errorf("SIM_MEAN_REPAIR: %w", err)
		}
	}
	if v := os.Getenv("SIM_MAX_CONCURRENT"); v != "" {
		if cfg.MaxConcurrent, err = strconv.Atoi(v); err != nil {
			return cfg, fmt.Errorf("SIM_MAX_CONCURRENT: %w", err)
		}
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
