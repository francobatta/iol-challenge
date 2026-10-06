// Package httpserver runs an HTTP server for as long as a context lasts.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// shutdownTimeout is how long requests in flight get to finish once the server stops.
const shutdownTimeout = 10 * time.Second

// Run serves handler on addr until ctx is cancelled, then lets requests in flight
// finish. It returns nil after a clean shutdown and an error if it could not serve.
func Run(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	// The goroutine ends when ListenAndServe returns, which Shutdown below guarantees.
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	select {
	case err := <-serveErr:
		return fmt.Errorf("serving on %s: %v", addr, err)
	case <-ctx.Done():
	}
	// ctx is cancelled by now, so requests in flight get a deadline of their own.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
