// Command worker sends the notifications queued for one provider.
//
// One worker serves every app that sends through its provider. Run more workers to send
// more at once: they share each queue through the broker. See package consume for how
// deliveries are handled.
//
// A worker has min(CONCURRENCY, PREFETCH x apps with deliveries waiting) requests in
// progress, so PREFETCH decides how much of a worker one app can use when it is the only
// one sending. A larger PREFETCH lets a lone app use more, but also means more of an
// app's deliveries are already in the worker's hands, where a high-priority delivery
// that arrives later cannot overtake them.
//
// It is configured through the environment; the config type lists the variables, and
// ../../.env has values for local development. dependencies.go shows what the worker is
// made of.
//
// A worker that loses its connection to RabbitMQ exits, and is expected to be restarted.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"

	"github.com/francobatta/iol-challenge/notification-system/commons/httpserver"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := parseConfig(env.ToMap(os.Environ()))
	if err != nil {
		return err
	}
	deps, err := newDependencies(ctx, cfg)
	if err != nil {
		return err
	}
	defer deps.close()

	// The pool and the metrics server stop together: whichever returns first cancels
	// the other.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poolErr := make(chan error, 1)
	go func() { defer cancel(); poolErr <- deps.pool.Run(ctx) }()
	metricsErr := make(chan error, 1)
	go func() { defer cancel(); metricsErr <- httpserver.Run(ctx, cfg.MetricsAddr, deps.opsRouter) }()

	slog.Info("Sending", "provider", cfg.Provider, "concurrency", cfg.Concurrency, "prefetch", cfg.Prefetch, "metrics_addr", cfg.MetricsAddr)
	return errors.Join(<-poolErr, <-metricsErr)
}
