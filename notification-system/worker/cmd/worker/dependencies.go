package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/francobatta/iol-challenge/notification-system/commons/telemetry"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/breaker"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/consume"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/provider"
)

const (
	// An app's breaker opens after this many failed sends in a row, and lets a probe
	// through after the cool-down.
	breakerThreshold = 5
	breakerCooldown  = 10 * time.Second
)

// dependencies is everything the worker is made of: the pool that consumes and sends,
// and the handler that serves its metrics.
type dependencies struct {
	mq          *amqp.Connection
	flushTraces func(context.Context) error

	pool *consume.Pool

	metricsRouter http.Handler
}

// newDependencies builds the worker from the bottom up: the provider and RabbitMQ
// first, then what handles a delivery, and the pool that feeds it. The caller must call
// close when it is done.
func newDependencies(ctx context.Context, cfg config) (_ *dependencies, err error) {
	d := &dependencies{}
	// Do not leave the connection open if a later step fails.
	defer func() {
		if err != nil {
			d.close()
		}
	}()

	// Clients and connections.
	sender, err := provider.New(cfg.Provider, cfg.ProviderURL)
	if err != nil {
		return nil, fmt.Errorf("PROVIDER: %v", err)
	}
	discoverer, err := consume.NewDiscoverer(cfg.RabbitMQAPIURL, cfg.Provider, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("RABBITMQ_API_URL: %v", err)
	}
	d.mq, err = amqp.Dial(cfg.AMQPURL)
	if err != nil {
		return nil, fmt.Errorf("AMQP_URL: connecting to RabbitMQ: %v", err)
	}
	publisher, err := consume.NewAMQPPublisher(d.mq)
	if err != nil {
		return nil, err
	}
	d.flushTraces, err = telemetry.SetupTracing(ctx, "notification-worker-"+cfg.Provider, cfg.OTLPEndpoint)
	if err != nil {
		return nil, err
	}

	// Metrics.
	registry := telemetry.NewRegistry()
	metrics := consume.NewMetrics(registry, cfg.Provider)
	d.metricsRouter = telemetry.MetricsRouter(registry)

	// Services.
	breakers := breaker.NewSet(breaker.Settings{
		Threshold: breakerThreshold,
		Cooldown:  breakerCooldown,
		// Only failures that say the provider is struggling. A notification it
		// refuses says something about the notification.
		IsFailure: func(err error) bool { return errors.Is(err, provider.ErrRetryable) },
		OnChange:  metrics.BreakerChanged,
	})
	handler := consume.NewHandler(sender, publisher, breakers, metrics, cfg.Concurrency)
	d.pool = consume.NewPool(d.mq, discoverer.Queues, handler, metrics, cfg.Prefetch, consume.DefaultDiscoveryInterval)
	return d, nil
}

func (d *dependencies) close() {
	if d.flushTraces != nil {
		// The context the worker ran under is cancelled by now, so flushing gets a
		// deadline of its own.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.flushTraces(ctx); err != nil {
			slog.Warn("Could not flush traces", "err", err)
		}
	}
	if d.mq != nil {
		// Closing the connection is what returns the deliveries the worker held but
		// never started to their queues.
		if err := d.mq.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			slog.Warn("Could not close the RabbitMQ connection", "err", err)
		}
	}
}
