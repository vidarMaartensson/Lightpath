package gateway

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
)

// fakeEvents is an events service whose stream the test drives: messages
// sent on msgs arrive at Recv, an error on fail ends the stream.
type fakeEvents struct {
	live     bool
	stateErr error
	watchErr error
	msgs     chan *pb.WatchStateResponse
	fail     chan error
	ctxs     chan context.Context // receives each stream's context
}

func newFakeEvents() *fakeEvents {
	return &fakeEvents{
		live: true,
		msgs: make(chan *pb.WatchStateResponse, 8),
		fail: make(chan error, 1),
		ctxs: make(chan context.Context, 8),
	}
}

func (f *fakeEvents) GetState(context.Context, *pb.GetStateRequest, ...grpc.CallOption) (*pb.GetStateResponse, error) {
	if f.stateErr != nil {
		return nil, f.stateErr
	}
	return &pb.GetStateResponse{State: &pb.LiveState{}, Live: f.live}, nil
}

func (f *fakeEvents) WatchState(ctx context.Context, _ *pb.WatchStateRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.WatchStateResponse], error) {
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	f.ctxs <- ctx
	return &fakeStream{ctx: ctx, f: f}, nil
}

type fakeStream struct {
	grpc.ClientStream
	ctx context.Context
	f   *fakeEvents
}

func (s *fakeStream) Context() context.Context { return s.ctx }
func (s *fakeStream) Recv() (*pb.WatchStateResponse, error) {
	select {
	case m := <-s.f.msgs:
		return m, nil
	case err := <-s.f.fail:
		return nil, err
	case <-s.ctx.Done():
		return nil, status.FromContextError(s.ctx.Err()).Err()
	}
}

func state(seq uint64, down ...string) *pb.WatchStateResponse {
	return &pb.WatchStateResponse{State: &pb.LiveState{Seq: seq, DownNodeIds: down}}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// liveServer serves /live through the full Handler, including the logging
// middleware, as in production.
func liveServer(t *testing.T, l *Live, writeTimeout time.Duration) *httptest.Server {
	t.Helper()
	if l.Log == nil {
		l.Log = quietLog()
	}
	srv := httptest.NewUnstartedServer(Handler(nil, l, "", quietLog()))
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// sseReader reads one SSE block (up to a blank line) at a time.
type sseReader struct{ r *bufio.Reader }

func (s sseReader) next(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended after %q: %v", b.String(), err)
		}
		if line == "\n" {
			return b.String()
		}
		b.WriteString(line)
	}
}

func openLive(t *testing.T, ctx context.Context, url string) (*http.Response, sseReader) {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/live", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp, sseReader{bufio.NewReader(resp.Body)}
}

func TestLiveRelaysStateAsServerSentEvents(t *testing.T) {
	events := newFakeEvents()
	events.msgs <- state(1, "acc-ali")
	srv := liveServer(t, &Live{Events: events}, 0)

	resp, sse := openLive(t, context.Background(), srv.URL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for header, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"X-Accel-Buffering": "no",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	if got := sse.next(t); got != "retry: 3000\n" {
		t.Errorf("first block = %q, want the retry interval", got)
	}
	want := "id: 1\nevent: state\ndata: {\"seq\":1,\"down\":[\"acc-ali\"]}\n"
	if got := sse.next(t); got != want {
		t.Errorf("snapshot:\n got %q\nwant %q", got, want)
	}

	update := state(2, "acc-ali", "cab-teg-1")
	update.Cause = &pb.Event{
		Seq: 2, Id: "sim-1-2", Kind: pb.EventKind_EVENT_KIND_SITE_DOWN, NodeId: "cab-teg-1",
		At: timestamppb.New(time.Date(2026, 10, 24, 14, 2, 0, 0, time.UTC)), Source: "simulator",
	}
	events.msgs <- update
	want = "id: 2\nevent: state\ndata: {\"seq\":2,\"down\":[\"acc-ali\",\"cab-teg-1\"]," +
		"\"cause\":{\"id\":\"sim-1-2\",\"kind\":\"SITE_DOWN\",\"nodeId\":\"cab-teg-1\"," +
		"\"at\":\"2026-10-24T14:02:00Z\",\"source\":\"simulator\"}}\n"
	if got := sse.next(t); got != want {
		t.Errorf("update:\n got %q\nwant %q", got, want)
	}
}

func TestLiveEmptyStateIsAnEmptyList(t *testing.T) {
	events := newFakeEvents()
	events.msgs <- state(0)
	srv := liveServer(t, &Live{Events: events}, 0)

	_, sse := openLive(t, context.Background(), srv.URL)
	sse.next(t) // retry
	if got := sse.next(t); !strings.Contains(got, `"down":[]`) {
		t.Fatalf("want down as [] so the browser need not check for null, got %q", got)
	}
}

func TestLiveDropsInvalidCauseButKeepsState(t *testing.T) {
	events := newFakeEvents()
	m := state(3, "acc-ali")
	m.Cause = &pb.Event{Id: "e", Kind: pb.EventKind_EVENT_KIND_SITE_DOWN, NodeId: "acc-ali"} // no time
	events.msgs <- m
	srv := liveServer(t, &Live{Events: events}, 0)

	_, sse := openLive(t, context.Background(), srv.URL)
	sse.next(t)
	want := "id: 3\nevent: state\ndata: {\"seq\":3,\"down\":[\"acc-ali\"]}\n"
	if got := sse.next(t); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLivePings(t *testing.T) {
	events := newFakeEvents()
	events.msgs <- state(1)
	srv := liveServer(t, &Live{Events: events, Ping: 10 * time.Millisecond}, 0)

	_, sse := openLive(t, context.Background(), srv.URL)
	sse.next(t)
	sse.next(t)
	if got := sse.next(t); got != ": ping\n" {
		t.Fatalf("got %q, want a ping comment", got)
	}
}

func TestLiveUnavailable(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *fakeEvents)
	}{
		{"events service down", func(f *fakeEvents) {
			f.watchErr = status.Error(codes.Unavailable, "connection refused")
		}},
		{"stream fails before the state", func(f *fakeEvents) {
			f.fail <- status.Error(codes.Unavailable, "connection reset")
		}},
		{"no state in time", func(f *fakeEvents) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := newFakeEvents()
			tt.setup(events)
			srv := liveServer(t, &Live{Events: events, FirstTimeout: 50 * time.Millisecond}, 0)

			resp, err := http.Get(srv.URL + "/live")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("status %d, want 503", resp.StatusCode)
			}
			if resp.Header.Get("Retry-After") == "" {
				t.Error("want Retry-After")
			}
			if strings.Contains(string(body), "connection") {
				t.Errorf("body leaks internals: %q", body)
			}
		})
	}
}

func TestLiveEndsWhenEventsStreamDies(t *testing.T) {
	events := newFakeEvents()
	events.msgs <- state(1)
	srv := liveServer(t, &Live{Events: events}, 0)

	resp, sse := openLive(t, context.Background(), srv.URL)
	sse.next(t)
	sse.next(t)
	events.fail <- status.Error(codes.Unavailable, "events restarted")

	// The response must end, so the browser reconnects.
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("want a clean end, got %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("unexpected data after the stream died: %q", rest)
	}
}

func TestLiveOutlivesWriteTimeout(t *testing.T) {
	events := newFakeEvents()
	events.msgs <- state(1)
	srv := liveServer(t, &Live{Events: events}, 100*time.Millisecond)

	_, sse := openLive(t, context.Background(), srv.URL)
	sse.next(t)
	sse.next(t)
	time.Sleep(300 * time.Millisecond)
	events.msgs <- state(2, "acc-ali")
	if got := sse.next(t); !strings.HasPrefix(got, "id: 2\n") {
		t.Fatalf("got %q after the write timeout, want the next update", got)
	}
}

func TestLiveStopsOnShutdown(t *testing.T) {
	events := newFakeEvents()
	events.msgs <- state(1)
	stop := make(chan struct{})
	srv := liveServer(t, &Live{Events: events, Stop: stop}, 0)

	resp, sse := openLive(t, context.Background(), srv.URL)
	sse.next(t)
	sse.next(t)
	close(stop)

	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(resp.Body); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("want a clean end, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open after stop")
	}
}

func TestLiveCancelsUpstreamWhenBrowserLeaves(t *testing.T) {
	events := newFakeEvents()
	events.msgs <- state(1)
	srv := liveServer(t, &Live{Events: events}, 0)

	ctx, cancel := context.WithCancel(context.Background())
	_, sse := openLive(t, ctx, srv.URL)
	sse.next(t)
	sse.next(t)
	upstream := <-events.ctxs
	cancel()

	select {
	case <-upstream.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchState stream left open after the browser went away")
	}
}

func TestNoLiveRouteWithoutEvents(t *testing.T) {
	srv := httptest.NewServer(Handler(nil, nil, "", quietLog()))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/live")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestLiveStatusQuery(t *testing.T) {
	tests := []struct {
		name   string
		events EventsClient
		want   string
	}{
		{"no events service configured", nil, "OFF"},
		{"live", &fakeEvents{live: true}, "ON"},
		{"demo mode", &fakeEvents{live: false}, "OFF"},
		{"events service down", &fakeEvents{stateErr: status.Error(codes.Unavailable, "down")}, "UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := NewSchema(fakeTopology{}, nil, tt.events)
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(Handler(schema, nil, "", quietLog()))
			defer srv.Close()

			res := post(t, srv.URL, `{ liveStatus }`, nil)
			if len(res.Errors) > 0 {
				t.Fatalf("errors: %+v", res.Errors)
			}
			if got := string(res.Data); got != `{"liveStatus":"`+tt.want+`"}` {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
