package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// testHandler is a minimal handler standing in for whatever a consumer supplies.
func testHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestServe_GracefulShutdown(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	srv := New(testLogger(), "127.0.0.1", 0, testHandler())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, 5*time.Second) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		must.NoError(err)
	case <-time.After(10 * time.Second):
		want.Fail("server shutdown timed out")
	}
}

func TestServe_StartError(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	// Occupy a port so the server's ListenAndServe fails deterministically.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must.NoError(err)
	t.Cleanup(func() { _ = listener.Close() })

	port := Port(listener.Addr().(*net.TCPAddr).Port)
	srv := New(testLogger(), "127.0.0.1", port, testHandler())

	err = srv.Serve(context.Background(), 5*time.Second)
	must.Error(err)
	want.ErrorIs(err, ErrServerStart)
}

func TestServe_ShutdownError(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must.NoError(err)

	srv := New(testLogger(), "127.0.0.1", 0, testHandler())
	go func() { _ = srv.http.Serve(listener) }()

	// Hold a connection open so shutdown cannot complete within the deadline.
	conn, err := net.Dial("tcp", listener.Addr().String())
	must.NoError(err)
	t.Cleanup(func() { _ = conn.Close() })

	time.Sleep(100 * time.Millisecond)

	err = srv.shutdown(time.Nanosecond)
	must.Error(err)
	want.ErrorIs(err, ErrServerShutdown)
}

func TestChooseError(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	// A startup error is preferred over any shutdown outcome.
	want.ErrorIs(chooseError(ErrServerStart, nil), ErrServerStart)
	want.ErrorIs(chooseError(ErrServerStart, ErrServerShutdown), ErrServerStart)
	// Absent a startup error, the shutdown outcome is reported.
	want.ErrorIs(chooseError(nil, ErrServerShutdown), ErrServerShutdown)
	want.NoError(chooseError(nil, nil))
}

// TestListenReportsItsTerminalResultExactlyOnce names listen's claim. Serve
// joins the listen goroutine by RECEIVING from this channel, so the count is
// load-bearing in both directions: sending nothing deadlocks Serve on shutdown,
// and sending twice leaks a goroutine blocked on a send nobody will ever
// receive — a leak per server lifetime, invisible until a long-running process
// exhausts something.
//
// The translation is the other half: a clean close (http.ErrServerClosed) is
// NOT a failure, and reporting it as one would make every graceful shutdown
// look like a crashed server.
func TestListenReportsItsTerminalResultExactlyOnce(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in      error
		wantErr error
		name    string
	}{
		{name: "a clean close is not a failure", in: http.ErrServerClosed, wantErr: nil},
		{name: "no error is not a failure", in: nil, wantErr: nil},
		{name: "a real failure becomes ErrServerStart", in: errBoom, wantErr: ErrServerStart},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := startError(tc.in)
			if tc.wantErr == nil {
				assert.NoError(t, got)
				return
			}
			assert.ErrorIs(t, got, tc.wantErr)
			assert.ErrorIs(t, got, tc.in, "the cause must stay reachable")
		})
	}
}

// TestStopPrefersAStartupFailureOverAShutdownOutcome names stop's and
// chooseError's claim. A server that failed to bind its port still gets shut
// down, and the shutdown will usually succeed — so returning the shutdown
// outcome would report success for a server that never started. The startup
// error wins whenever there is one.
func TestStopPrefersAStartupFailureOverAShutdownOutcome(t *testing.T) {
	t.Parallel()

	startFailed := ErrServerStart.With(errBoom)
	shutdownFailed := ErrServerShutdown.With(errBoom)

	assert.ErrorIs(t, chooseError(startFailed, nil), ErrServerStart,
		"a startup failure with a clean shutdown must still report the startup failure")
	assert.ErrorIs(t, chooseError(startFailed, shutdownFailed), ErrServerStart,
		"and must not be masked when the shutdown also failed")
	assert.ErrorIs(t, chooseError(nil, shutdownFailed), ErrServerShutdown,
		"with no startup failure, the shutdown outcome is the answer")
	assert.NoError(t, chooseError(nil, nil), "a clean run reports nothing")
}

// errBoom is an arbitrary underlying failure, used to prove a cause stays
// reachable through the sentinels.
var errBoom = errors.New("boom")
