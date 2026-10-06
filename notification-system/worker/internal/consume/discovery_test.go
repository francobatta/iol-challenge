package consume

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// A fakeManagementAPI answers queue listings the way RabbitMQ's management API does,
// one queue name per page.
type fakeManagementAPI struct {
	queues []string
	status int // answered instead of a listing, if not zero

	gotQueries []url.Values
	gotUser    string
	gotPass    string
}

func (f *fakeManagementAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/queues" {
		http.NotFound(w, r)
		return
	}
	f.gotQueries = append(f.gotQueries, r.URL.Query())
	f.gotUser, f.gotPass, _ = r.BasicAuth()
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	// A missing or malformed page stays 0, which matches no queue.
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	items := ""
	if page >= 1 && page <= len(f.queues) {
		items = fmt.Sprintf(`{"name": %q}`, f.queues[page-1])
	}
	fmt.Fprintf(w, `{"items": [%s], "page": %d, "page_count": %d}`, items, page, len(f.queues))
}

func newTestDiscoverer(t *testing.T, api *fakeManagementAPI) *Discoverer {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	d, err := NewDiscoverer(strings.Replace(srv.URL, "http://", "http://user:secret@", 1), "twilio", srv.Client())
	if err != nil {
		t.Fatalf("Setup: NewDiscoverer failed: %v", err)
	}
	return d
}

func TestQueues(t *testing.T) {
	api := &fakeManagementAPI{queues: []string{
		"notify.send.twilio.app-1",
		"notify.send.twilio-backup.app-9", // another provider whose name starts the same
		"notify.send.twilio.app-2",
	}}
	d := newTestDiscoverer(t, api)

	got, err := d.Queues(t.Context())
	if err != nil {
		t.Fatalf("Queues failed: %v", err)
	}
	want := []string{"notify.send.twilio.app-1", "notify.send.twilio.app-2"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Queues returned unexpected diff (-want +got):\n%s", diff)
	}

	if len(api.gotQueries) != 3 {
		t.Fatalf("Queues made %d requests for 3 pages, want 3", len(api.gotQueries))
	}
	q := api.gotQueries[0]
	if q.Get("name") != `^notify\.send\.twilio\.` || q.Get("use_regex") != "true" {
		t.Errorf("Queues asked for name %q with use_regex %q, want the provider's prefix as a regex", q.Get("name"), q.Get("use_regex"))
	}
	if api.gotUser != "user" || api.gotPass != "secret" {
		t.Errorf("Queues authenticated as %q with password %q, want the credentials of the API URL", api.gotUser, api.gotPass)
	}
}

func TestQueuesWithNoQueuesYet(t *testing.T) {
	d := newTestDiscoverer(t, &fakeManagementAPI{})
	got, err := d.Queues(t.Context())
	if err != nil || len(got) != 0 {
		t.Errorf("Queues(with no send queues) = %q, %v, want none, nil", got, err)
	}
}

func TestQueuesReportsAPIFailure(t *testing.T) {
	d := newTestDiscoverer(t, &fakeManagementAPI{status: http.StatusUnauthorized})
	if _, err := d.Queues(t.Context()); err == nil {
		t.Error("Queues(with the API answering 401) = nil error, want an error")
	}
}

func TestNewDiscovererRejectsBadURL(t *testing.T) {
	for _, bad := range []string{"", "rabbitmq:15672", "://x"} {
		if _, err := NewDiscoverer(bad, "twilio", http.DefaultClient); err == nil {
			t.Errorf("NewDiscoverer(%q) = nil error, want an error", bad)
		}
	}
}
