package httpserver

// errs holds this package's intrinsic sentinels on the shared go-error mechanism.
import errs "github.com/gomatic/go-error"

const (
	// ErrServerStart indicates the server could not start listening.
	ErrServerStart errs.Const = "failed to start server"
	// ErrServerShutdown indicates the server did not shut down within the deadline.
	ErrServerShutdown errs.Const = "failed to shutdown server"
)
