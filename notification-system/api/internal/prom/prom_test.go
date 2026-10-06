package prom_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/prom"
)

// newClient returns a Client for a server that answers every query with body, and a
// function that returns the path and form of the last request the server got.
func newClient(t *testing.T, status int, body string) (*prom.Client, func() map[string]string) {
	t.Helper()
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("Setup: parsing the request to %s: %v", r.URL.Path, err)
		}
		got = map[string]string{"path": r.URL.Path}
		for name := range r.Form {
			got[name] = r.Form.Get(name)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("Setup: writing the response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := prom.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("Setup: NewClient(%q) failed: %v", srv.URL, err)
	}
	return c, func() map[string]string { return got }
}

func TestInstant(t *testing.T) {
	// The second series has no value JSON could carry.
	const body = `{"status": "success", "data": {"resultType": "vector", "result": [
		{"metric": {"provider": "twilio"}, "value": [1000, "12.5"]},
		{"metric": {"provider": "fcm"}, "value": [1000, "NaN"]}
	]}}`
	c, request := newClient(t, http.StatusOK, body)

	got, err := c.Instant(t.Context(), "up", time.Unix(1000, 0))
	if err != nil {
		t.Fatalf("Instant(%q) failed: %v", "up", err)
	}
	want := []insight.Sample{{Labels: map[string]string{"provider": "twilio"}, Value: 12.5}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Instant(%q) mismatch (-want +got):\n%s", "up", diff)
	}
	wantRequest := map[string]string{"path": "/api/v1/query", "query": "up", "time": "1000"}
	if diff := cmp.Diff(wantRequest, request()); diff != "" {
		t.Errorf("Instant(%q) sent an unexpected request (-want +got):\n%s", "up", diff)
	}
}

func TestRange(t *testing.T) {
	const body = `{"status": "success", "data": {"resultType": "matrix", "result": [
		{"metric": {"provider": "twilio"}, "values": [[1000, "1"], [1015, "+Inf"], [1030, "3"]]}
	]}}`
	c, request := newClient(t, http.StatusOK, body)

	got, err := c.Range(t.Context(), "up", time.Unix(1000, 0), time.Unix(1030, 0), 15*time.Second)
	if err != nil {
		t.Fatalf("Range(%q) failed: %v", "up", err)
	}
	want := []insight.Series{{Labels: map[string]string{"provider": "twilio"}, Points: []insight.Point{{1000, 1}, {1030, 3}}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Range(%q) mismatch (-want +got):\n%s", "up", diff)
	}
	wantRequest := map[string]string{"path": "/api/v1/query_range", "query": "up", "start": "1000", "end": "1030", "step": "15"}
	if diff := cmp.Diff(wantRequest, request()); diff != "" {
		t.Errorf("Range(%q) sent an unexpected request (-want +got):\n%s", "up", diff)
	}
}

func TestQueryRejected(t *testing.T) {
	const body = `{"status": "error", "errorType": "bad_data", "error": "parse error"}`
	c, _ := newClient(t, http.StatusBadRequest, body)
	if _, err := c.Instant(t.Context(), "up{", time.Unix(1000, 0)); err == nil {
		t.Errorf("Instant(%q) succeeded on a 400 response, want an error", "up{")
	}
	if _, err := c.Range(t.Context(), "up{", time.Unix(1000, 0), time.Unix(1030, 0), 15*time.Second); err == nil {
		t.Errorf("Range(%q) succeeded on a 400 response, want an error", "up{")
	}
}
