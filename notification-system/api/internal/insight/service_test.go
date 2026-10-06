package insight_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight/insighttest"
)

const testAppID = "app-1"

// A prometheus answers queries from what a test put in it. The key of each map is a
// fragment of a query; a query gets the answer of the longest fragment it contains, and
// no series if it contains none.
type prometheus struct {
	instant map[string][]insight.Sample
	ranged  map[string][]insight.Series
}

func answer[T any](answers map[string][]T, query string) []T {
	var best string
	for fragment := range answers {
		if strings.Contains(query, fragment) && len(fragment) > len(best) {
			best = fragment
		}
	}
	return answers[best]
}

// newService returns a Service over a Querier that answers from p, and a function that
// returns the queries it has been asked.
func newService(t *testing.T, p prometheus) (_ *insight.Service, asked func() []string) {
	t.Helper()
	var (
		mu      sync.Mutex // queries arrive from several goroutines
		queries []string
	)
	record := func(query string) {
		mu.Lock()
		defer mu.Unlock()
		queries = append(queries, query)
	}

	q := insighttest.NewMockQuerier(gomock.NewController(t))
	q.EXPECT().Instant(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, query string, _ time.Time) ([]insight.Sample, error) {
			record(query)
			return answer(p.instant, query), nil
		})
	q.EXPECT().Range(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, query string, _, _ time.Time, _ time.Duration) ([]insight.Series, error) {
			record(query)
			return answer(p.ranged, query), nil
		})
	return insight.NewService(q), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(queries)
	}
}

// sample returns a Sample with the given value and labels, which are names and values
// in turn.
func sample(value float64, labels ...string) insight.Sample {
	s := insight.Sample{Labels: map[string]string{}, Value: value}
	for i := 0; i+1 < len(labels); i += 2 {
		s.Labels[labels[i]] = labels[i+1]
	}
	return s
}

func TestSnapshotApp(t *testing.T) {
	rate := []insight.Series{{Labels: map[string]string{"provider": "twilio"}, Points: []insight.Point{{100, 2}, {105, 3}}}}
	svc, _ := newService(t, prometheus{
		instant: map[string][]insight.Sample{
			"increase(notify_fanout_deliveries_total": {sample(40, "provider", "twilio"), sample(7, "provider", "fcm")},
			"increase(notify_deliveries_total": {
				sample(30, "provider", "twilio", "outcome", "sent"),
				sample(4, "provider", "twilio", "outcome", "retried"),
				sample(2, "provider", "twilio", "outcome", "failed"),
				sample(7, "provider", "fcm", "outcome", "sent"),
			},
			"(notify_fanout_deliveries_total{": {sample(100, "provider", "twilio"), sample(7, "provider", "fcm")},
			`outcome=~"sent|failed"`:           {sample(92, "provider", "twilio"), sample(9, "provider", "fcm")},
			"messages_ready":                   {sample(5, "provider", "twilio")},
			"messages_unacked":                 {sample(3, "provider", "twilio")},
			"notify_breaker_open":              {sample(1, "provider", "twilio"), sample(0, "provider", "fcm")},
		},
		ranged: map[string][]insight.Series{"rate(notify_fanout_deliveries_total": rate},
	})

	got, err := svc.Snapshot(t.Context(), testAppID, 15*time.Minute)
	if err != nil {
		t.Fatalf("Snapshot(%q, 15m) failed: %v", testAppID, err)
	}
	want := insight.App{
		Providers: []insight.ProviderDeliveries{
			{Provider: "twilio", Queued: 40, Sent: 30, Retried: 4, Failed: 2, InTransit: 8, Ready: 5, Unacked: 3, BreakerOpen: true},
			{Provider: "mailchimp"},
			{Provider: "apns"},
			// More done than queued, as after a restart: not a negative number in transit.
			{Provider: "fcm", Queued: 7, Sent: 7},
		},
		QueuedRate:  rate,
		OutcomeRate: []insight.Series{},
		Backlog:     []insight.Series{},
	}
	if diff := cmp.Diff(want, got.App); diff != "" {
		t.Errorf("Snapshot(%q, 15m).App mismatch (-want +got):\n%s", testAppID, diff)
	}
	if got.WindowSeconds != 900 || got.StepSeconds != 15 {
		t.Errorf("Snapshot(%q, 15m) has window %ds and step %ds, want 900s and 15s", testAppID, got.WindowSeconds, got.StepSeconds)
	}
}

func TestSnapshotSystem(t *testing.T) {
	svc, _ := newService(t, prometheus{
		instant: map[string][]insight.Sample{
			"up": {
				sample(1, "job", "worker", "instance", "10.0.0.2:9090"),
				sample(0, "job", "rabbitmq", "instance", "rabbitmq:15692"),
				sample(1, "job", "api", "instance", "api:9090"),
			},
			"go_goroutines":                                {sample(12, "instance", "api:9090"), sample(40, "instance", "10.0.0.2:9090")},
			"process_resident_memory_bytes":                {sample(2048, "instance", "api:9090")},
			"process_cpu_seconds_total":                    {sample(0.5, "instance", "api:9090")},
			"count by (provider)":                          {sample(2, "provider", "twilio")},
			"sum by (provider) (notify_sends_in_flight)":   {sample(9, "provider", "twilio")},
			"max by (provider) (notify_queues_subscribed)": {sample(3, "provider", "twilio")},
			"sum by (queue)":                               {sample(1, "queue", "notify.dead")},
		},
	})

	got, err := svc.Snapshot(t.Context(), testAppID, time.Hour)
	if err != nil {
		t.Fatalf("Snapshot(%q, 1h) failed: %v", testAppID, err)
	}
	none := []insight.Series{}
	want := insight.System{
		Targets: []insight.Target{
			{Job: "api", Instance: "api:9090", Up: true, Goroutines: 12, MemoryBytes: 2048, CPU: 0.5},
			{Job: "rabbitmq", Instance: "rabbitmq:15692"},
			{Job: "worker", Instance: "10.0.0.2:9090", Up: true, Goroutines: 40},
		},
		Pools: []insight.WorkerPool{
			{Provider: "twilio", Workers: 2, SendsInFlight: 9, QueuesSubscribed: 3},
			{Provider: "mailchimp"},
			{Provider: "apns"},
			{Provider: "fcm"},
		},
		Queues:     []insight.Sample{sample(1, "queue", "notify.dead")},
		LatencyP50: none, LatencyP95: none, LatencyP99: none,
		SendsInFlight: none, FanoutPagesRate: none, FanoutRejectedRate: none,
	}
	if diff := cmp.Diff(want, got.System); diff != "" {
		t.Errorf("Snapshot(%q, 1h).System mismatch (-want +got):\n%s", testAppID, diff)
	}
}

// A query that names an app-scoped metric must select the app's own series, or one app
// would see another's.
func TestSnapshotAsksOnlyForTheAppsSeries(t *testing.T) {
	// An ID that would end the label matcher early if it were not quoted.
	const (
		appID   = `a"} or up{x="`
		matcher = `app_id="a\"} or up{x=\""`
	)
	svc, asked := newService(t, prometheus{})
	if _, err := svc.Snapshot(t.Context(), appID, 5*time.Minute); err != nil {
		t.Fatalf("Snapshot(%q, 5m) failed: %v", appID, err)
	}

	perApp := []string{
		"notify_fanout_deliveries_total", "notify_deliveries_total", "notify_breaker_open",
		"messages_ready", "messages_unacked", "rabbitmq_detailed_queue_messages{app_id",
	}
	scoped := 0
	for _, query := range asked() {
		if !slices.ContainsFunc(perApp, func(metric string) bool { return strings.Contains(query, metric) }) {
			continue
		}
		scoped++
		if !strings.Contains(query, matcher) {
			t.Errorf("Snapshot(%q, 5m) asked %q, which does not select %s", appID, query, matcher)
		}
	}
	if want := 10; scoped != want {
		t.Errorf("Snapshot(%q, 5m) asked %d queries about the app, want %d; asked %q", appID, scoped, want, asked())
	}
}

func TestSnapshotErrors(t *testing.T) {
	failing := insighttest.NewMockQuerier(gomock.NewController(t))
	failing.EXPECT().Instant(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, errors.New("connection refused"))
	failing.EXPECT().Range(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)

	tests := []struct {
		name   string
		svc    *insight.Service
		window time.Duration
		want   error
	}{
		{name: "WindowTooShort", svc: insight.NewService(failing), window: time.Second, want: audience.ErrInvalid},
		{name: "WindowTooLong", svc: insight.NewService(failing), window: 25 * time.Hour, want: audience.ErrInvalid},
		{name: "NoPrometheus", svc: insight.NewService(nil), window: time.Hour, want: insight.ErrUnavailable},
		{name: "PrometheusDoesNotAnswer", svc: insight.NewService(failing), window: time.Hour, want: insight.ErrUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.svc.Snapshot(t.Context(), testAppID, test.window); !errors.Is(err, test.want) {
				t.Errorf("Snapshot(%q, %s) error = %v, want one wrapping %v", testAppID, test.window, err, test.want)
			}
		})
	}
}
