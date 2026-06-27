package httpserver

// Imported bare (the package is named error); this file declares only sentinels
// and uses no builtin error type, so each declaration reads errs.Const.
import errs "github.com/gomatic/go-error"

const (
	// ErrServerStart indicates the server could not start listening.
	ErrServerStart errs.Const = "failed to start server"
	// ErrServerShutdown indicates the server did not shut down within the deadline.
	ErrServerShutdown errs.Const = "failed to shutdown server"
)
