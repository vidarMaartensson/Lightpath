// Command gateway serves the GraphQL API and the built frontend.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/gateway"
	"github.com/vidarMaartensson/Lightpath/internal/rpc"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "gateway")
	if err := run(log); err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("LISTEN_ADDR", ":8080")

	topoConn, err := rpc.Dial(env("TOPOLOGY_ADDR", "localhost:9001"))
	if err != nil {
		return err
	}
	defer topoConn.Close()
	impactConn, err := rpc.Dial(env("IMPACT_ADDR", "localhost:9002"))
	if err != nil {
		return err
	}
	defer impactConn.Close()

	schema, err := gateway.NewSchema(
		pb.NewTopologyServiceClient(topoConn),
		pb.NewImpactServiceClient(impactConn),
	)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           gateway.Handler(schema, os.Getenv("STATIC_DIR"), log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
