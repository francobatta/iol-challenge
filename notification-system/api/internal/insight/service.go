package insight

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/commons/providers"
)

// The windows a snapshot may cover.
const (
	MinWindow = time.Minute
	MaxWindow = 24 * time.Hour
)

const (
	// pointsPerSeries keeps a chart the same width whatever the window.
	pointsPerSeries = 60
	// scrapeInterval is how often Prometheus collects; a finer step would repeat values.
	scrapeInterval = 5 * time.Second
	// minRateWindow spans enough scrapes for rate() to have something to work with.
	minRateWindow = 30 * time.Second
)

// A Service answers with snapshots of the metrics in Prometheus.
type Service struct {
	prom Querier
}

// NewService returns a Service that reads from prom. With a nil prom every snapshot
// fails with ErrUnavailable.
func NewService(prom Querier) *Service {
	return &Service{prom: prom}
}

// Snapshot returns the metrics of the app and of the system over the window that ends
// now. Series about an app are those of appID only.
//
// It returns audience.ErrInvalid if window is not between MinWindow and MaxWindow, and
// ErrUnavailable if Prometheus cannot be asked.
func (s *Service) Snapshot(ctx context.Context, appID string, window time.Duration) (Snapshot, error) {
	if window < MinWindow || window > MaxWindow {
		return Snapshot{}, fmt.Errorf("%w: range must be from %s to %s", audience.ErrInvalid, MinWindow, MaxWindow)
	}
	if s.prom == nil {
		return Snapshot{}, fmt.Errorf("%w: no Prometheus is configured", ErrUnavailable)
	}

	end := time.Now()
	start := end.Add(-window)
	step := max(window/pointsPerSeries, scrapeInterval).Truncate(time.Second)
	snap := Snapshot{At: end, WindowSeconds: int(window.Seconds()), StepSeconds: int(step.Seconds())}

	var (
		app = "app_id=" + strconv.Quote(appID)
		w   = promDuration(window)
		rw  = promDuration(max(step, minRateWindow))
	)
	const deadQueue = `queue="notify.dead"`

	// Every query runs at once; each writes to a variable of its own.
	g, ctx := errgroup.WithContext(ctx)
	instant := func(dst *[]Sample, query string) {
		g.Go(func() (err error) {
			*dst, err = s.prom.Instant(ctx, query, end)
			return err
		})
	}
	ranged := func(dst *[]Series, query string) {
		g.Go(func() error {
			series, err := s.prom.Range(ctx, query, start, end, step)
			if series == nil {
				series = []Series{} // encode as [] rather than null
			}
			*dst = series
			return err
		})
	}

	var queued, outcomes, everQueued, everDone, ready, unacked, breaker []Sample
	instant(&queued, "sum by (provider) ("+growth("notify_fanout_deliveries_total{"+app+"}", w)+")")
	instant(&outcomes, "sum by (provider, outcome) ("+growth("notify_deliveries_total{"+app+"}", w)+")")
	instant(&everQueued, "sum by (provider) (notify_fanout_deliveries_total{"+app+"})")
	instant(&everDone, `sum by (provider) (notify_deliveries_total{`+app+`,outcome=~"sent|failed"})`)
	instant(&ready, "sum by (provider) (rabbitmq_detailed_queue_messages_ready{"+app+"})")
	instant(&unacked, "sum by (provider) (rabbitmq_detailed_queue_messages_unacked{"+app+"})")
	instant(&breaker, "max by (provider) (notify_breaker_open{"+app+"})")
	ranged(&snap.App.QueuedRate, "sum by (provider) (rate(notify_fanout_deliveries_total{"+app+"}["+rw+"]))")
	ranged(&snap.App.OutcomeRate, "sum by (outcome) (rate(notify_deliveries_total{"+app+"}["+rw+"]))")
	ranged(&snap.App.Backlog, "sum by (provider) (rabbitmq_detailed_queue_messages{"+app+"})")

	var up, goroutines, memory, cpu, workers, inFlight, subscribed []Sample
	instant(&up, "up")
	instant(&goroutines, "go_goroutines")
	instant(&memory, "process_resident_memory_bytes")
	instant(&cpu, "rate(process_cpu_seconds_total["+rw+"])")
	instant(&workers, "count by (provider) (notify_queues_subscribed)")
	instant(&inFlight, "sum by (provider) (notify_sends_in_flight)")
	instant(&subscribed, "max by (provider) (notify_queues_subscribed)")
	instant(&snap.System.Queues, "sum by (queue) (rabbitmq_detailed_queue_messages{"+deadQueue+"})")
	latency := func(quantile string) string {
		return "histogram_quantile(" + quantile + ", sum by (provider, le) (rate(notify_provider_request_seconds_bucket[" + rw + "])))"
	}
	ranged(&snap.System.LatencyP50, latency("0.5"))
	ranged(&snap.System.LatencyP95, latency("0.95"))
	ranged(&snap.System.LatencyP99, latency("0.99"))
	ranged(&snap.System.SendsInFlight, "sum by (provider) (notify_sends_in_flight)")
	ranged(&snap.System.FanoutPagesRate, "sum(rate(notify_fanout_pages_total["+rw+"]))")
	ranged(&snap.System.FanoutRejectedRate, "sum(rate(notify_fanout_rejected_total["+rw+"]))")

	if err := g.Wait(); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	const provider = "provider"
	outcome := func(name string) map[string]float64 {
		named := slices.DeleteFunc(slices.Clone(outcomes), func(s Sample) bool { return s.Labels["outcome"] != name })
		return valuesBy(named, provider)
	}
	for _, name := range providers.Names() {
		snap.App.Providers = append(snap.App.Providers, ProviderDeliveries{
			Provider:    name,
			Queued:      valuesBy(queued, provider)[name],
			Sent:        outcome("sent")[name],
			Retried:     outcome("retried")[name],
			Failed:      outcome("failed")[name],
			Undecodable: outcome("undecodable")[name],
			// The counters of a worker that restarted start again from zero, which can
			// take the difference below it.
			InTransit:   max(0, valuesBy(everQueued, provider)[name]-valuesBy(everDone, provider)[name]),
			Ready:       valuesBy(ready, provider)[name],
			Unacked:     valuesBy(unacked, provider)[name],
			BreakerOpen: valuesBy(breaker, provider)[name] > 0,
		})
		snap.System.Pools = append(snap.System.Pools, WorkerPool{
			Provider:         name,
			Workers:          valuesBy(workers, provider)[name],
			SendsInFlight:    valuesBy(inFlight, provider)[name],
			QueuesSubscribed: valuesBy(subscribed, provider)[name],
		})
	}

	snap.System.Targets = targets(up, goroutines, memory, cpu)
	if snap.System.Queues == nil {
		snap.System.Queues = []Sample{}
	}
	slices.SortFunc(snap.System.Queues, func(a, b Sample) int { return cmp.Compare(a.Labels["queue"], b.Labels["queue"]) })
	return snap, nil
}

// targets returns what Prometheus scrapes, by job and instance, with what each target
// says about its own process. A target that is not a Go program says nothing.
func targets(up, goroutines, memory, cpu []Sample) []Target {
	const instance = "instance"
	var (
		goroutinesBy = valuesBy(goroutines, instance)
		memoryBy     = valuesBy(memory, instance)
		cpuBy        = valuesBy(cpu, instance)
	)
	targets := make([]Target, 0, len(up))
	for _, s := range up {
		at := s.Labels[instance]
		targets = append(targets, Target{
			Job:         s.Labels["job"],
			Instance:    at,
			Up:          s.Value > 0,
			Goroutines:  goroutinesBy[at],
			MemoryBytes: memoryBy[at],
			CPU:         cpuBy[at],
		})
	}
	slices.SortFunc(targets, func(a, b Target) int {
		return cmp.Or(cmp.Compare(a.Job, b.Job), cmp.Compare(a.Instance, b.Instance))
	})
	return targets
}

// valuesBy returns the value of each sample under the value of its label.
func valuesBy(samples []Sample, label string) map[string]float64 {
	values := make(map[string]float64, len(samples))
	for _, s := range samples {
		values[s.Labels[label]] = s.Value
	}
	return values
}

// growth returns PromQL for how much each of the counters that selector selects grew
// over the window.
//
// increase() alone measures from a series' first sample in the window, and a counter is
// first seen already holding whatever it counted before that scrape. So a series that
// did not exist when the window began, as on the day an app first sends, is taken at
// its value now: all of it was counted inside the window.
func growth(selector, window string) string {
	return "(increase(" + selector + "[" + window + "]) and " + selector + " offset " + window + ") or " + selector
}

// promDuration writes d the way PromQL reads it.
func promDuration(d time.Duration) string {
	return strconv.Itoa(int(d.Seconds())) + "s"
}
