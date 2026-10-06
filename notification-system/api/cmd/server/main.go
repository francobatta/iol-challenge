// Command server runs the audience and notification REST API, and next to it the
// goroutines that dispatch the notifications it accepts. See package dispatch for those.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	"golang.org/x/sync/errgroup"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/dispatch"
	"github.com/francobatta/iol-challenge/notification-system/commons/httpserver"
)

// fanoutConcurrency is how many jobs one server fans out at a time.
const fanoutConcurrency = 4

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
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

	// Every goroutine ends when ctx is cancelled, and one that fails cancels the others.
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return httpserver.Run(ctx, cfg.Addr, deps.router) })
	g.Go(func() error { return httpserver.Run(ctx, cfg.MetricsAddr, deps.opsRouter) })
	g.Go(func() error {
		deps.fanout.Run(ctx, fanoutConcurrency, dispatch.DefaultPollInterval)
		return nil
	})
	slog.Info("Listening", "addr", cfg.Addr, "metrics_addr", cfg.MetricsAddr)
	return g.Wait()
}
