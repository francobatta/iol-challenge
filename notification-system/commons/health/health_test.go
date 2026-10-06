package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/francobatta/iol-challenge/notification-system/commons/health"
)

func TestProbes(t *testing.T) {
	notReady := func(context.Context) error { return errors.New("database is down") }
	ready := func(context.Context) error { return nil }

	tests := []struct {
		name  string
		ready func(context.Context) error
		path  string
		want  int
	}{
		{name: "AliveWhileNotReady", ready: notReady, path: "/healthz", want: http.StatusOK},
		{name: "Ready", ready: ready, path: "/readyz", want: http.StatusOK},
		{name: "NotReady", ready: notReady, path: "/readyz", want: http.StatusServiceUnavailable},
		{name: "ReadyWithNothingToCheck", ready: nil, path: "/readyz", want: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := chi.NewRouter()
			health.Mount(r, test.ready)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, test.path, nil))
			if rec.Code != test.want {
				t.Errorf("GET %s = %d, want %d", test.path, rec.Code, test.want)
			}
		})
	}
}
