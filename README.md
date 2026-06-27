# go-httpserver

A minimal, stdlib-only HTTP server lifecycle wrapper with context-driven graceful shutdown.

## Install

```sh
go get github.com/gomatic/go-httpserver
```

## Usage

The caller wires cancellation — typically with [`signal.NotifyContext`](https://pkg.go.dev/os/signal#NotifyContext) — and passes the resulting context to `Serve`. When the context is cancelled (e.g. on SIGINT/SIGTERM), the server shuts down gracefully within the supplied timeout.

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gomatic/go-httpserver"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httpserver.New(slog.Default(), "127.0.0.1", 8080, handler)
	if err := srv.Serve(ctx, 5*time.Second); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
```

`Serve` blocks until the context is cancelled or startup fails. Request contexts derive from the lifecycle context (via `http.Server.BaseContext`), so in-flight handlers observe the same cancellation. A pending startup failure is preferred over a clean shutdown, so a real error is never masked.
