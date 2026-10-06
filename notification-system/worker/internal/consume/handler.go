// Package consume is the worker's loop: it takes deliveries from the send queues of one
// provider, sends them, and counts what became of each.
//
// A [Pool] finds the provider's queues, one per app, and subscribes to them. A
// [Handler] decides the fate of each delivery:
//
//   - sent: it is done with.
//   - failed in a way that may pass (see provider.ErrRetryable): it is published to a
//     retry tier, which returns it to its queue after a delay that grows with each
//     attempt.
//   - refused by the provider, or out of retries: it is published to the dead queue.
//
// A delivery that was not sent is acknowledged only after the broker has confirmed what
// was published in its place, so it is in some queue at every moment and a worker that
// dies loses nothing. The price is that a delivery can be sent twice.
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
	"github.com/francobatta/iol-challenge/notification-system/commons/topology"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/breaker"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/provider"
)

// MaxRetries is how many times a delivery is retried before it is given up on. Its Nth
// retry goes through the Nth retry tier, which is what makes the backoff exponential.
func MaxRetries() int { return len(topology.RetryTiers()) }

// publishTimeout bounds publishing what replaces a delivery. It applies even during
// shutdown, when a delivery that has failed still has to be put somewhere.
const publishTimeout = 10 * time.Second

// A Sender delivers a notification through a provider. Its errors wrap
// provider.ErrRetryable or provider.ErrPermanent.
type Sender interface {
	Send(ctx context.Context, d message.Delivery) error
}

// A Publisher puts messages on the broker. Its methods return once the broker has
// confirmed the message.
type Publisher interface {
	// PublishRetry sends msg back to its queue after the delay of the given retry
	// tier, counting one more attempt.
	PublishRetry(ctx context.Context, tier int, msg Message) error
	// PublishDead sets msg aside with the reason it was given up on.
	PublishDead(ctx context.Context, msg Message, reason string) error
}

// A Message is a delivery as it came off a queue.
type Message struct {
	Queue    string // the queue it came from, and returns to on a retry
	Body     []byte
	Attempt  int   // how many times it has been retried
	Priority uint8 // carried over on a retry
	// Headers are the message's string headers, among them its trace context.
	Headers map[string]string
}

// A Handler processes deliveries. It is safe for concurrent use.
type Handler struct {
	sender   Sender
	pub      Publisher
	breakers *breaker.Set
	slots    chan struct{} // one token per send in flight
	metrics  *Metrics
}

// NewHandler returns a Handler that sends through sender, at most concurrency
// deliveries at a time, and publishes those that fail on pub. The breakers are keyed by
// app.
func NewHandler(sender Sender, pub Publisher, breakers *breaker.Set, metrics *Metrics, concurrency int) *Handler {
	return &Handler{sender: sender, pub: pub, breakers: breakers, slots: make(chan struct{}, concurrency), metrics: metrics}
}

// Handle processes one delivery and reports whether it is done with: true if msg can be
// acknowledged, false if it must go back to its queue to be handled again.
//
// Cancelling ctx makes Handle give up on a delivery it has not started to send. One it
// is sending is seen through, including publishing it again if it failed.
func (h *Handler) Handle(ctx context.Context, msg Message) (done bool) {
	// What has been started is finished even if the worker is shutting down.
	finish := context.WithoutCancel(ctx)

	var d message.Delivery
	if err := json.Unmarshal(msg.Body, &d); err != nil {
		// It names no app, so it is counted under none.
		slog.ErrorContext(ctx, "Setting aside an undecodable delivery", "queue", msg.Queue, "err", err)
		return h.replace(finish, d, outcomeUndecodable, func(ctx context.Context) error { return h.pub.PublishDead(ctx, msg, "undecodable") })
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
		// Nothing replaces a delivery that was sent, so there is nothing to publish.
		h.metrics.deliveries.WithLabelValues(d.AppID, outcomeSent).Inc()
		log.DebugContext(ctx, "Delivered")
		return true

	case !started:
		// The worker is shutting down and this delivery was still waiting for its turn.
		return false

	case errors.Is(err, provider.ErrRetryable) && msg.Attempt < MaxRetries():
		span.SetStatus(codes.Error, err.Error())
		log.WarnContext(ctx, "Delivery failed; it will be retried", "err", err)
		return h.replace(finish, d, outcomeRetried, func(ctx context.Context) error { return h.pub.PublishRetry(ctx, msg.Attempt, msg) })

	default:
		span.SetStatus(codes.Error, err.Error())
		log.ErrorContext(ctx, "Delivery failed for good", "err", err)
		reason := "refused by the provider"
		if errors.Is(err, provider.ErrRetryable) {
			reason = "out of retries"
		}
		return h.replace(finish, d, outcomeFailed, func(ctx context.Context) error { return h.pub.PublishDead(ctx, msg, reason) })
	}
}

// replace publishes what takes the place of a delivery that was not sent, under
// publishTimeout, and reports whether it succeeded. Only then is the outcome counted: a
// failure means the delivery must be handled again, and would be counted twice.
func (h *Handler) replace(ctx context.Context, d message.Delivery, outcome string, publish func(context.Context) error) bool {
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := publish(ctx); err != nil {
		slog.ErrorContext(ctx, "Could not publish a delivery that was not sent; it will be handled again",
			"app_id", d.AppID, "job_id", d.JobID, "message_id", d.MessageID, "err", err)
		return false
	}
	h.metrics.deliveries.WithLabelValues(d.AppID, outcome).Inc()
	return true
}

// The outcomes a delivery is counted under.
const (
	outcomeSent        = "sent"        // the provider accepted it
	outcomeRetried     = "retried"     // it failed and will be tried again
	outcomeFailed      = "failed"      // it will not be tried again
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
			Help:        "Deliveries handled, by app and outcome: sent, retried, failed or undecodable.",
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
