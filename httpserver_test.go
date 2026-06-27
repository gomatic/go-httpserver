package httpserver

import (
	"context"
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

func TestNew(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	h := testHandler()
	srv := New(testLogger(), "127.0.0.1", 8080, h)
	want.Equal("127.0.0.1:8080", srv.Addr())
	want.NotNil(srv.Handler())
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
