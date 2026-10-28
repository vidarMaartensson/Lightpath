package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"

	pb "github.com/vidarMaartensson/Lightpath/gen/lightpath/v1"
	"github.com/vidarMaartensson/Lightpath/internal/rpc"
)

// EventsClient is the subset of pb.EventServiceClient the gateway uses.
type EventsClient interface {
	GetState(ctx context.Context, in *pb.GetStateRequest, opts ...grpc.CallOption) (*pb.GetStateResponse, error)
	WatchState(ctx context.Context, in *pb.WatchStateRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.WatchStateResponse], error)
}

const (
	defaultPing         = 20 * time.Second
	defaultFirstTimeout = 5 * time.Second
	// reconnectDelay is sent to the browser as the SSE retry interval.
	reconnectDelay = 3 * time.Second
)

// Live relays the events service's WatchState stream to the browser as
// server-sent events. It translates and computes nothing: the browser asks
// for the numbers through the outage query, like the sandbox does.
type Live struct {
	Events EventsClient
	Log    *slog.Logger
	// Stop ends every open stream when closed. http.Server.Shutdown waits
	// for connections to go idle, which a stream never does.
	Stop <-chan struct{}
	// Ping is how often a comment is sent so proxies keep the connection
	// open. FirstTimeout is how long to wait for the events service before
	// answering 503. Zero means the default; set shorter in tests.
	Ping         time.Duration
	FirstTimeout time.Duration
}

func (l *Live) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// The status code can only be chosen before anything is written, so
	// wait for the first message (the full state) before committing to
	// 200. Until then, any failure is a 503.
	firstTimeout := orDefault(l.FirstTimeout, defaultFirstTimeout)
	timer := time.AfterFunc(firstTimeout, cancel)
	stream, err := l.Events.WatchState(ctx, &pb.WatchStateRequest{})
	var first *pb.WatchStateResponse
	if err == nil {
		first, err = stream.Recv()
	}
	// If the timer fired, ctx is cancelled and the stream is unusable even
	// if the first message made it in just in time.
	if !timer.Stop() {
		err = fmt.Errorf("no state from the events service within %s", firstTimeout)
	}
	if err != nil {
		l.Log.Warn("live view unavailable", "err", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "The live view is not responding. Try again shortly; the sandbox still works.\n")
		return
	}

	rc := http.NewResponseController(w)
	// The server's WriteTimeout is right for ordinary requests and would
	// cut this one off, so lift it for this response only.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		l.Log.Warn("live: cannot lift write deadline", "err", err)
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(s string) bool {
		if _, err := io.WriteString(w, s); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send(fmt.Sprintf("retry: %d\n\n", reconnectDelay.Milliseconds())) || !send(l.event(first)) {
		return
	}

	// Recv blocks, so it runs on its own goroutine and hands messages over.
	updates := make(chan *pb.WatchStateResponse)
	failed := make(chan error, 1)
	go func() {
		for {
			m, err := stream.Recv()
			if err != nil {
				failed <- err
				return
			}
			select {
			case updates <- m:
			case <-ctx.Done():
				return
			}
		}
	}()

	ping := time.NewTicker(orDefault(l.Ping, defaultPing))
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return // the browser left
		case <-l.Stop:
			return // shutting down; the browser reconnects to the next gateway
		case err := <-failed:
			// Ending the response makes the browser reconnect, and the new
			// stream starts with the full state.
			l.Log.Warn("live: events stream ended", "err", err)
			return
		case m := <-updates:
			if !send(l.event(m)) {
				return
			}
		case <-ping.C:
			if !send(": ping\n\n") {
				return
			}
		}
	}
}

// liveMessage is the JSON in each SSE event.
type liveMessage struct {
	Seq   uint64     `json:"seq"`
	Down  []string   `json:"down"`
	Cause *liveCause `json:"cause,omitempty"`
}

type liveCause struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	NodeID string `json:"nodeId"`
	At     string `json:"at"`
	Source string `json:"source"`
}

// event formats one update as an SSE event. A cause that does not pass
// validation is dropped and logged; the state itself is still sent, since
// it is what the map draws.
func (l *Live) event(m *pb.WatchStateResponse) string {
	msg := liveMessage{Seq: m.GetState().GetSeq(), Down: m.GetState().GetDownNodeIds()}
	if msg.Down == nil {
		msg.Down = []string{}
	}
	if m.GetCause() != nil {
		e, err := rpc.EventFromProto(m.GetCause())
		if err != nil {
			l.Log.Warn("live: dropping invalid cause", "err", err)
		} else {
			msg.Cause = &liveCause{
				ID:     e.ID,
				Kind:   string(e.Kind),
				NodeID: e.NodeID,
				At:     e.At.UTC().Format(time.RFC3339),
				Source: e.Source,
			}
		}
	}
	data, _ := json.Marshal(msg) // only strings and numbers; cannot fail
	// JSON has no raw newlines, so it fits on one data line.
	return fmt.Sprintf("id: %d\nevent: state\ndata: %s\n\n", msg.Seq, strings.TrimSpace(string(data)))
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}
