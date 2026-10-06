// Command mockprovider stands in for Twilio, Mailchimp, APNs and FCM in development
// and load tests. It accepts what the worker's provider clients send, takes as long to
// answer as a real provider might, and fails as often as it is told to.
//
// Each request waits a random time between 1 millisecond and 1 second, uniformly
// distributed, so the mean is about half a second.
//
// It is configured through the environment; the config type lists the variables, and
// ../../.env has values for local development. dependencies.go shows what the mock is
// made of.
package main

import (
	"context"
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
		fmt.Fprintln(os.Stderr, "mockprovider:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := parseConfig(env.ToMap(os.Environ()))
	if err != nil {
		return err
	}
	deps := newDependencies(cfg)

	slog.Info("Listening", "addr", cfg.Addr, "error_rate", cfg.ErrorRate, "throttle_rate", cfg.ThrottleRate)
	return httpserver.Run(ctx, cfg.Addr, deps.router)
}
