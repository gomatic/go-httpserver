# go-httpserver

A small HTTP server with graceful-shutdown lifecycle (package `httpserver`): `New(logger, host, port, handler)` → `Serve(ctx, timeout)` with signal-aware shutdown and a start/shutdown error contract. **Handler-agnostic** — the caller supplies the `http.Handler`. Generic; lives in `gomatic`.

- Owns its sentinels (`ErrServerStart`, `ErrServerShutdown`) on `gomatic/go-error` (`error.Const`).
- Gate: gofumpt, vet, staticcheck, govulncheck, gocognit ≤ 7, 100% coverage.
