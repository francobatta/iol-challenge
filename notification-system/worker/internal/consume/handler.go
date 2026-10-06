// Package consume is the worker's loop: it takes deliveries from the send queues of one
// provider, sends them, and counts what became of each.
//
// A [Pool] finds the provider's queues, one per app, and subscribes to them. A
// [Handler] decides the fate of each delivery:
//
//   - sent: it is done with.
//   - failed in a way that may pass (see provider.ErrRetryable): it is rejected back to
//     its queue, where the broker holds it for a delay that grows with each failure,
//     and moves it to the dead queue once it has failed too often.
//   - refused by the provider: it is rejected for good, and the broker moves it to the
//     dead queue.
//
// The worker publishes nothing: retrying and dead-lettering are the broker's, set up on
// the send queues by package topology. A delivery never leaves the broker until it is
// sent, so a worker that dies loses nothing. The price is that a delivery can be sent
// twice.
//
// The outcomes are not reported to anyone: they are counted in [Metrics], by app, which
// together with the queue metrics of RabbitMQ is how the state of an app on a provider
// is known.
package consume

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/francobatta/iol-challenge/notification-system/commons/message"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/breaker"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/provider"
)

// A Sender delivers a notification through a provider. Its errors wrap
// provider.ErrRetryable or provider.ErrPermanent.
type Sender interface {
	Send(ctx context.Context, d message.Delivery) error
}

// A Message is a delivery as it came off a queue.
type Message struct {
	Queue   string // the queue it came from
	Body    []byte
	Attempt int // how many deliveries of it have failed before
	// Headers are the message's string headers, among them its trace context.
	Headers map[string]string
}

// A Verdict is what is to become of a delivery. It is carried out by settling the
// delivery with the broker.
type Verdict int

const (
	// Sent: the delivery is done with, and is acknowledged.
	Sent Verdict = iota
	// Retry: the send failed in a way that may pass. The delivery is rejected back to
	// its queue, which counts as a failed delivery.
	Retry
	// Dead: the delivery will never be sent. It is rejected for good.
	Dead
	// NotStarted: the delivery was not attempted. It goes back to its queue as it was.
	NotStarted
)

// A Handler processes deliveries. It is safe for concurrent use.
type Handler struct {
	sender   Sender
	breakers *breaker.Set
	slots    chan struct{} // one token per send in flight
	metrics  *Metrics
}

// NewHandler returns a Handler that sends through sender, at most concurrency
// deliveries at a time. The breakers are keyed by app.
func NewHandler(sender Sender, breakers *breaker.Set, metrics *Metrics, concurrency int) *Handler {
	return &Handler{sender: sender, breakers: breakers, slots: make(chan struct{}, concurrency), metrics: metrics}
}

// Handle processes one delivery and returns what is to become of it.
//
// Cancelling ctx makes Handle give up on a delivery it has not started to send. One it
// is sending is seen through.
func (h *Handler) Handle(ctx context.Context, msg Message) Verdict {
	// What has been started is finished even if the worker is shutting down.
	finish := context.WithoutCancel(ctx)

	var d message.Delivery
	if err := json.Unmarshal(msg.Body, &d); err != nil {
		// It names no app, so it is counted under none.
		slog.ErrorContext(ctx, "Setting aside an undecodable delivery", "queue", msg.Queue, "err", err)
		h.metrics.deliveries.WithLabelValues(d.AppID, outcomeUndecodable).Inc()
		return Dead
	}

	finish, span := otel.Tracer("worker").Start(extractContext(finish, msg.Headers), "deliver")
	defer span.End()
	span.SetAttributes(
		attribute.String("app_id", d.AppID),
		attribute.String("job_id", d.JobID),
		attribute.String("message_id", d.MessageID),
		attribute.Int("attempt", msg.Attempt),
	)

	var started bool
	err := h.breakers.Do(ctx, d.AppID, func() error {
		// Checked first because a select with both cases ready picks either.
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case h.slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-h.slots }()
		started = true
		h.metrics.inFlight.Inc()
		defer h.metrics.inFlight.Dec()
		start := time.Now()
		err := h.sender.Send(finish, d)
		h.metrics.latency.Observe(time.Since(start).Seconds())
		return err
	})

	log := slog.With("app_id", d.AppID, "job_id", d.JobID, "message_id", d.MessageID, "attempt", msg.Attempt)
	switch {
	case err == nil:
		h.metrics.deliveries.WithLabelValues(d.AppID, outcomeSent).Inc()
		log.DebugContext(ctx, "Delivered")
		return Sent

	case !started:
		// The worker is shutting down and this delivery was still waiting for its turn.
		return NotStarted

	case errors.Is(err, provider.ErrRetryable):
		span.SetStatus(codes.Error, err.Error())
		log.WarnContext(ctx, "Delivery failed; the broker will retry it or give up on it", "err", err)
		h.metrics.deliveries.WithLabelValues(d.AppID, outcomeRetried).Inc()
		return Retry

	default:
		span.SetStatus(codes.Error, err.Error())
		log.ErrorContext(ctx, "Delivery failed for good", "err", err)
		h.metrics.deliveries.WithLabelValues(d.AppID, outcomeFailed).Inc()
		return Dead
	}
}

// The outcomes a delivery is counted under. A delivery that runs out of retries is not
// among them: the broker gives up on it, so it shows as a message in the dead queue.
const (
	outcomeSent        = "sent"        // the provider accepted it
	outcomeRetried     = "retried"     // it failed in a way that may pass and was handed back to the broker
	outcomeFailed      = "failed"      // the provider refused it
	outcomeUndecodable = "undecodable" // it could not be read, and names no app
)

// Metrics is what a worker reports. Every metric carries the worker's provider, and
// those about an app its app_id, so that summed over the workers they give the state of
// each app on each provider.
type Metrics struct {
	deliveries  *prometheus.CounterVec
	latency     prometheus.Histogram
	inFlight    prometheus.Gauge
	breakerOpen *prometheus.GaugeVec
	queues      prometheus.Gauge
}

func NewMetrics(reg prometheus.Registerer, provider string) *Metrics {
	labels := prometheus.Labels{"provider": provider}
	m := &Metrics{
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "notify_deliveries_total",
			Help:        "Deliveries handled, by app and outcome: sent, retried (handed back to the broker), failed (refused by the provider) or undecodable.",
			ConstLabels: labels,
		}, []string{"app_id", "outcome"}),
		latency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        "notify_provider_request_seconds",
			Help:        "Duration of requests to the provider.",
			ConstLabels: labels,
			Buckets:     []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1, 2, 3},
		}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "notify_sends_in_flight",
			Help:        "Requests to the provider in progress.",
			ConstLabels: labels,
		}),
		breakerOpen: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "notify_breaker_open",
			Help:        "Whether the circuit breaker of an app is open in this worker: 1 if it is, 0 if not.",
			ConstLabels: labels,
		}, []string{"app_id"}),
		queues: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "notify_queues_subscribed",
			Help:        "Send queues this worker is consuming.",
			ConstLabels: labels,
		}),
	}
	reg.MustRegister(m.deliveries, m.latency, m.inFlight, m.breakerOpen, m.queues)
	return m
}

// BreakerChanged records that the breaker of an app opened or closed. It has the
// signature of breaker.Settings.OnChange.
func (m *Metrics) BreakerChanged(appID string, open bool) {
	if open {
		m.breakerOpen.WithLabelValues(appID).Set(1)
		slog.Warn("Circuit breaker opened; pausing the app's deliveries", "app_id", appID)
		return
	}
	m.breakerOpen.WithLabelValues(appID).Set(0)
	slog.Info("Circuit breaker closed; resuming the app's deliveries", "app_id", appID)
}
