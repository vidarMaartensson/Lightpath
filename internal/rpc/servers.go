package rpc

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/impactsvc"
	"github.com/vidarMaartensson/Lightpath/internal/topology"
)

// TopologyServer serves topology snapshots from a store.
type TopologyServer struct {
	pb.UnimplementedTopologyServiceServer
	Store topology.Store
}

// GetNetwork implements pb.TopologyServiceServer.
func (s *TopologyServer) GetNetwork(ctx context.Context, _ *pb.GetNetworkRequest) (*pb.GetNetworkResponse, error) {
	n, err := s.Store.Network(ctx)
	if err != nil {
		return nil, toStatus(err, codes.Unavailable)
	}
	return &pb.GetNetworkResponse{Network: NetworkToProto(n)}, nil
}

// ImpactServer serves outage analyses.
type ImpactServer struct {
	pb.UnimplementedImpactServiceServer
	Service *impactsvc.Service
}

// AnalyzeOutage implements pb.ImpactServiceServer.
func (s *ImpactServer) AnalyzeOutage(ctx context.Context, req *pb.AnalyzeOutageRequest) (*pb.AnalyzeOutageResponse, error) {
	var a impactsvc.Analysis
	var err error
	if b := req.GetBaseline(); b != nil {
		a, err = s.Service.AnalyzeChange(ctx, b.GetFailedNodeIds(), req.GetFailedNodeIds())
	} else {
		a, err = s.Service.Analyze(ctx, req.GetFailedNodeIds())
	}
	switch {
	case errors.Is(err, impactsvc.ErrInvalidInput):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, impactsvc.ErrTopologyUnavailable):
		return nil, status.Error(codes.Unavailable, err.Error())
	case err != nil:
		return nil, toStatus(err, codes.Internal)
	}
	return AnalysisToProto(a), nil
}

// toStatus keeps context cancellation and deadlines distinguishable from
// real failures, which matters for retries and alerting.
func toStatus(err error, fallback codes.Code) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Error(fallback, err.Error())
}

// LoggingInterceptor logs every unary call with its duration and code.
func LoggingInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err)
		level := slog.LevelInfo
		if code != codes.OK && code != codes.InvalidArgument && code != codes.Canceled {
			level = slog.LevelWarn
		}
		log.LogAttrs(ctx, level, "rpc",
			slog.String("method", info.FullMethod),
			slog.String("code", code.String()),
			slog.Duration("duration", time.Since(start)),
		)
		return resp, err
	}
}
