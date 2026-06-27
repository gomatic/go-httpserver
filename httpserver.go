// Package httpserver provides a small HTTP server with graceful-shutdown
// lifecycle management.
//
// It owns the start/stop machinery — listen, signal-aware shutdown, and the
// start/shutdown error contract — and nothing else: the caller supplies the
// http.Handler. It holds no CLI or orchestration logic and is reusable by any
// service that needs to serve HTTP and stop cleanly.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type (
	// Host is the address the server binds to.
	Host string
	// Port is the TCP port the server listens on.
	Port int
)

// Server wraps an *http.Server with lifecycle management. It uses a pointer
// receiver because it owns non-copyable server machinery.
type Server struct {
	http   *http.Server
	logger *slog.Logger
}

// readHeaderTimeout bounds how long the server waits for request headers,
// preventing Slowloris-style attacks that hold connections open by trickling
// headers (gosec G112).
const readHeaderTimeout = 10 * time.Second

// New builds a Server bound to host:port serving the supplied handler.
func New(logger *slog.Logger, host Host, port Port, handler http.Handler) *Server {
	return &Server{
		http: &http.Server{
			Addr:              address(host, port),
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
		},
		logger: logger,
	}
}

// Handler returns the server's HTTP handler.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// Addr returns the configured listen address.
func (s *Server) Addr() string { return s.http.Addr }

// Serve starts the server and blocks until ctx is cancelled or startup fails,
// then shuts down gracefully within timeout.
func (s *Server) Serve(ctx context.Context, timeout time.Duration) error {
	errs := make(chan error, 1)
	go s.listen(errs)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return s.shutdown(timeout)
	}
}

// listen runs ListenAndServe, reporting any non-shutdown startup error.
func (s *Server) listen(errs chan<- error) {
	s.logger.Info("Server starting.", "address", s.http.Addr)
	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs <- ErrServerStart.With(err)
	}
}

// shutdown attempts a graceful shutdown bounded by timeout.
func (s *Server) shutdown(timeout time.Duration) error {
	s.logger.Info("Shutdown signal received, starting graceful shutdown.")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.http.Shutdown(ctx); err != nil {
		return ErrServerShutdown.With(err)
	}
	s.logger.Info("Server stopped gracefully.")
	return nil
}

// address formats host and port as a listen address.
func address(host Host, port Port) string {
	return fmt.Sprintf("%s:%d", string(host), int(port))
}
