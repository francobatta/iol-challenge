package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/broker"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/dispatch"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/httpapi"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/postgres"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/token"
	"github.com/francobatta/iol-challenge/notification-system/commons/telemetry"
)

// dependencies is everything the server is made of: the API it serves, the background
// work it runs next to it, and the handler that reports on that work.
type dependencies struct {
	db          *pgxpool.Pool
	mq          *broker.Client
	flushTraces func(context.Context) error

	router        http.Handler
	metricsRouter http.Handler

	fanout *dispatch.Fanout
}

// newDependencies builds the server from the bottom up: connections first, then the
// repository over them, and the services and transport over that. The caller must call
// close when it is done.
func newDependencies(ctx context.Context, cfg config) (_ *dependencies, err error) {
	d := &dependencies{}
	// Do not leave some of the connections open if a later one fails.
	defer func() {
		if err != nil {
			d.close()
		}
	}()

	// Connections.
	d.db, err = pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL: %v", err)
	}
	if err := d.db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("cannot reach the database at DATABASE_URL: %v", err)
	}
	d.mq, err = broker.Dial(cfg.AMQPURL)
	if err != nil {
		return nil, fmt.Errorf("AMQP_URL: %v", err)
	}
	d.flushTraces, err = telemetry.SetupTracing(ctx, "notification-api", cfg.OTLPEndpoint)
	if err != nil {
		return nil, err
	}
	tokens, err := token.NewSigner(cfg.JWTSecret)
	if err != nil {
		return nil, err
	}

	// Metrics.
	registry := telemetry.NewRegistry()
	metrics := dispatch.NewMetrics(registry)
	d.metricsRouter = telemetry.MetricsRouter(registry)

	// Repository.
	repo := postgres.NewRepository(d.db)

	// Services.
	audiences := audience.NewService(repo)
	notifications := notify.NewService(repo)
	d.fanout = dispatch.NewFanout(repo, d.mq, metrics, dispatch.DefaultPageSize, dispatch.DefaultLease)

	// Transport.
	d.router = httpapi.NewRouter(audiences, notifications, tokens, cfg.AdminKey)
	return d, nil
}

func (d *dependencies) close() {
	if d.flushTraces != nil {
		// The context the server ran under is cancelled by now, so flushing gets a
		// deadline of its own.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.flushTraces(ctx); err != nil {
			slog.Warn("Could not flush traces", "err", err)
		}
	}
	if d.mq != nil {
		if err := d.mq.Close(); err != nil {
			slog.Warn("Could not close the RabbitMQ connection", "err", err)
		}
	}
	if d.db != nil {
		d.db.Close()
	}
}
