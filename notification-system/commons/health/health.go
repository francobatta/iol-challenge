// Package health serves the two probes an orchestrator such as Kubernetes asks a
// process:
//
//	GET /healthz  is the process alive? Failing it gets the process restarted.
//	GET /readyz   can it do its work right now? Failing it takes the process out of
//	              rotation until it answers again.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// readyTimeout is how long a readiness check gets. It is shorter than the second a
// kubelet waits for an answer by default, so that the probe says why it failed instead
// of timing out.
const readyTimeout = 800 * time.Millisecond

// Mount adds the probes to r. ready reports why the process cannot do its work at the
// moment, or nil if it can; a nil ready means the process is ready whenever it is alive.
func Mount(r chi.Router, ready func(context.Context) error) {
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
			defer cancel()
			if err := ready(ctx); err != nil {
				slog.WarnContext(ctx, "Not ready", "err", err)
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}
