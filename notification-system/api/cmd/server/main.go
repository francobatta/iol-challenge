// Command server runs the audience REST API.
//
// It is configured through the environment:
//
//	DATABASE_URL  PostgreSQL connection string (required)
//	JWT_SECRET    secret that signs app tokens (required)
//	ADMIN_KEY     key that authorizes creating apps (required)
//	ADDR          address to listen on (default ":8080")
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/api"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/postgres"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/token"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}

// run serves the API until ctx is cancelled.
func run(ctx context.Context) error {
	databaseURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	jwtSecret, err := requireEnv("JWT_SECRET")
	if err != nil {
		return err
	}
	adminKey, err := requireEnv("ADMIN_KEY")
	if err != nil {
		return err
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("cannot reach the database at DATABASE_URL: %v", err)
	}

	tokens, err := token.NewSigner(jwtSecret)
	if err != nil {
		return err
	}
	svc := audience.NewService(postgres.NewStore(pool))
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.NewHandler(svc, tokens, adminKey),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// The goroutine ends when ListenAndServe returns, which Shutdown below guarantees.
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	slog.Info("Listening", "addr", addr)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	slog.Info("Shutting down")
	// ctx is already cancelled, so in-flight requests get a fresh deadline to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func requireEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("environment variable %s is required", name)
	}
	return value, nil
}
