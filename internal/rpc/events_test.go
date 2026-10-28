package rpc

import (
	"context"
	"io"
	"log/slog"
	"net"
	"slices"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/events"
	"github.com/vidarMaartensson/Lightpath/internal/eventsvc"
	"github.com/vidarMaartensson/Lightpath/internal/network"
)

type seedTopology struct{}

func (seedTopology) Network(context.Context) (*network.Network, bool, error) {
	return network.Seed(), false, nil
}

func newEventService(opts ...eventsvc.Option) *eventsvc.Service {
	quiet := eventsvc.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	return eventsvc.New(seedTopology{}, append([]eventsvc.Option{quiet}, opts...)...)
}

// dialEvents serves svc on an in-memory listener and returns a client.
func dialEvents(t *testing.T, svc *eventsvc.Service) pb.EventServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv, _ := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	pb.RegisterEventServiceServer(srv, &EventServer{Service: svc})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(serviceConfig),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return pb.NewEventServiceClient(conn)
}

func down(id, node string) events.Event {
	return events.Event{ID: id, Kind: events.SiteDown, NodeID: node, At: time.Unix(100, 0).UTC(), Source: "test"}
}

func TestWatchStateSendsStateFirstThenChanges(t *testing.T) {
	svc := newEventService()
	ctx := context.Background()
	if err := svc.Publish(ctx, down("e1", "acc-ali")); err != nil {
		t.Fatal(err)
	}
	client := dialEvents(t, svc)

	// Long enough to prove the unary timeout in serviceConfig does not
	// apply to the stream.
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stream, err := client.WatchState(streamCtx, &pb.WatchStateRequest{})
	if err != nil {
		t.Fatal(err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if first.GetCause() != nil {
		t.Errorf("first message has a cause: %v", first.GetCause())
	}
	if got := first.GetState().GetDownNodeIds(); !slices.Equal(got, []string{"acc-ali"}) {
		t.Errorf("first state = %v, want [acc-ali]", got)
	}

	if err := svc.Publish(ctx, down("e2", "cab-teg-1")); err != nil {
		t.Fatal(err)
	}
	second, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	cause, err := EventFromProto(second.GetCause())
	if err != nil {
		t.Fatalf("cause does not convert back: %v", err)
	}
	if cause.ID != "e2" || cause.Seq != 2 || second.GetState().GetSeq() != 2 {
		t.Errorf("cause = %+v, state seq = %d; want e2 at seq 2", cause, second.GetState().GetSeq())
	}
}

func TestWatchStateOutlivesUnaryTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits past the 3s unary timeout")
	}
	svc := newEventService()
	stream, err := dialEvents(t, svc).WatchState(context.Background(), &pb.WatchStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3500 * time.Millisecond)
	if err := svc.Publish(context.Background(), down("e1", "acc-ali")); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("stream ended after the unary timeout: %v", err)
	}
}

func TestWatchStateUnsubscribesWhenClientLeaves(t *testing.T) {
	svc := newEventService()
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := dialEvents(t, svc).WatchState(ctx, &pb.WatchStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	if n := svc.Subscribers(); n != 1 {
		t.Fatalf("%d subscribers, want 1", n)
	}
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for svc.Subscribers() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("subscription leaked after the client left")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestGetState(t *testing.T) {
	svc := newEventService()
	if err := svc.Publish(context.Background(), down("e1", "acc-ali")); err != nil {
		t.Fatal(err)
	}
	resp, err := dialEvents(t, svc).GetState(context.Background(), &pb.GetStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetState(); got.GetSeq() != 1 || !slices.Equal(got.GetDownNodeIds(), []string{"acc-ali"}) {
		t.Fatalf("state = %v", got)
	}
	if resp.GetLive() {
		t.Fatal("live = true for a server not marked live")
	}
}

func TestGetStateReportsLive(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterEventServiceServer(srv, &EventServer{Service: newEventService(), Live: true})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	resp, err := pb.NewEventServiceClient(conn).GetState(context.Background(), &pb.GetStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetLive() {
		t.Fatal("live = false for a live server")
	}
}

// stuckStream is a WatchState stream whose client stopped reading: Send
// blocks until release is closed.
type stuckStream struct {
	grpc.ServerStream
	ctx     context.Context
	sent    chan *pb.WatchStateResponse
	release chan struct{}
}

func (s *stuckStream) Context() context.Context { return s.ctx }
func (s *stuckStream) Send(m *pb.WatchStateResponse) error {
	s.sent <- m
	<-s.release
	return nil
}

func TestWatchStateDropsSlowClient(t *testing.T) {
	svc := newEventService(eventsvc.WithBuffer(2))
	stream := &stuckStream{
		ctx:     context.Background(),
		sent:    make(chan *pb.WatchStateResponse, 1),
		release: make(chan struct{}),
	}
	done := make(chan error)
	go func() { done <- (&EventServer{Service: svc}).WatchState(&pb.WatchStateRequest{}, stream) }()

	<-stream.sent // the snapshot, now stuck in Send
	for i, node := range []string{"acc-ali", "acc-teg", "acc-bol"} {
		if err := svc.Publish(context.Background(), down(string(rune('a'+i)), node)); err != nil {
			t.Fatal(err)
		}
	}
	close(stream.release)
	go func() {
		for range stream.sent {
		}
	}()

	err := <-done
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want UNAVAILABLE so the client reconnects, got %v", err)
	}
	if n := svc.Subscribers(); n != 0 {
		t.Fatalf("%d subscribers left, want 0", n)
	}
}
