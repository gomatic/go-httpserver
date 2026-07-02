// Package httpserver provides a small HTTP server with context-driven
// graceful-shutdown lifecycle management.
//
// It owns the start/stop machinery — listen, context-cancellation shutdown,
// and the start/shutdown error contract — and nothing else: the caller
// supplies the http.Handler and wires the cancellation (typically via
// signal.NotifyContext), passing the resulting context to Serve. It holds no
// CLI or orchestration logic and is reusable by any service that needs to
// serve HTTP and stop cleanly.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type (
	// Host is the address the server binds to.
	Host string
	// Port is the TCP port the server listens on.
	Port int
)

// Server wraps an *http.Server with lifecycle management. It is a value type:
// the non-copyable server machinery lives behind the http pointer field, so
// copies share the same underlying server.
type Server struct {
	http   *http.Server
	logger *slog.Logger
}

// readHeaderTimeout bounds how long the server waits for request headers,
// preventing Slowloris-style attacks that hold connections open by trickling
// headers (gosec G112).
const readHeaderTimeout = 10 * time.Second

// New builds a Server bound to host:port serving the supplied handler.
func New(logger *slog.Logger, host Host, port Port, handler http.Handler) Server {
	return Server{
		http: &http.Server{
			Addr:              address(host, port),
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
		},
		logger: logger,
	}
}

// Serve starts the server and blocks until ctx is cancelled or startup fails,
// then shuts down gracefully within timeout. Request contexts derive from ctx
// (via http.Server.BaseContext), so they observe the same lifecycle
// cancellation. When ctx is cancelled, a pending startup failure is preferred
// over reporting a clean shutdown, so a real error is never masked.
func (s Server) Serve(ctx context.Context, timeout time.Duration) error {
	s.http.BaseContext = func(net.Listener) context.Context { return ctx }

	errs := make(chan error, 1)
	go s.listen(errs)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return s.stop(timeout, errs)
	}
}

// listen runs ListenAndServe and reports its terminal result exactly once,
// translating a non-shutdown failure into ErrServerStart and a clean close
// into nil. The single send lets Serve join this goroutine.
func (s Server) listen(errs chan<- error) {
	s.logger.Info("Server starting.", "address", s.http.Addr)
	errs <- startError(s.http.ListenAndServe())
}

// stop shuts the server down, then joins the listen goroutine by receiving its
// terminal result. A pending startup error is preferred over the shutdown
// outcome so a real failure is never masked by a clean shutdown.
func (s Server) stop(timeout time.Duration, errs <-chan error) error {
	shutdownErr := s.shutdown(timeout)
	return chooseError(<-errs, shutdownErr)
}

// shutdown attempts a graceful shutdown bounded by timeout.
func (s Server) shutdown(timeout time.Duration) error {
	s.logger.Info("Context cancelled, starting graceful shutdown.")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.http.Shutdown(ctx); err != nil {
		return ErrServerShutdown.With(err)
	}
	s.logger.Info("Server stopped gracefully.")
	return nil
}

// startError translates a ListenAndServe result into the package contract: a
// clean shutdown (http.ErrServerClosed) is success; anything else is a wrapped
// ErrServerStart.
func startError(err error) error {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return ErrServerStart.With(err)
	}
	return nil
}

// chooseError prefers a startup error over a shutdown error, so a real startup
// failure is never masked by a clean (or merely slower) shutdown.
func chooseError(startErr, shutdownErr error) error {
	if startErr != nil {
		return startErr
	}
	return shutdownErr
}

// address formats host and port as a listen address.
func address(host Host, port Port) string {
	return fmt.Sprintf("%s:%d", string(host), int(port))
}
