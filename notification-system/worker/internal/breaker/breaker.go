// Package breaker keeps a worker from hammering a provider account that is failing.
//
// Each key, here an app, has a circuit breaker of its own. While a key's calls succeed
// they run freely. After several failures in a row the breaker opens and that key's
// calls wait instead of running; after a cool-down one call is let through as a probe,
// and its outcome closes the breaker or opens it for another cool-down. Other keys are
// unaffected throughout.
package breaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
)

type Settings struct {
	// Threshold is the number of consecutive failures that opens a breaker.
	Threshold uint32
	// Cooldown is how long a breaker stays open before letting a probe through.
	Cooldown time.Duration
	// IsFailure reports whether an error returned by a call counts against the
	// breaker. Errors that say nothing about the health of what is being called,
	// such as a request it rightly refused, should not.
	IsFailure func(error) bool
	// OnChange, if not nil, is called when the breaker of key opens, and when it
	// closes again. A probe that fails leaves it open, so that is not a change.
	OnChange func(key string, open bool)
}

// A Set holds one breaker per key, created on first use. It is safe for concurrent use.
type Set struct {
	settings Settings

	mu       sync.Mutex
	breakers map[string]*gobreaker.CircuitBreaker[struct{}]
}

func NewSet(s Settings) *Set {
	return &Set{settings: s, breakers: make(map[string]*gobreaker.CircuitBreaker[struct{}])}
}

func (s *Set) breaker(key string) *gobreaker.CircuitBreaker[struct{}] {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.breakers[key]; ok {
		return b
	}
	b := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        key,
		MaxRequests: 1, // probes let through after the cool-down
		Timeout:     s.settings.Cooldown,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= s.settings.Threshold },
		IsSuccessful: func(err error) bool {
			return err == nil || !s.settings.IsFailure(err)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			if s.settings.OnChange == nil {
				return
			}
			// Half-open, while the probe runs, still counts as open: only leaving
			// or reaching the closed state is a change.
			switch {
			case from == gobreaker.StateClosed:
				s.settings.OnChange(name, true)
			case to == gobreaker.StateClosed:
				s.settings.OnChange(name, false)
			}
		},
	})
	s.breakers[key] = b
	return b
}

// Do runs fn under the breaker of key and returns its error. While the breaker is
// open, or its probe is in progress, Do waits; it gives up and returns ctx's error if
// ctx is done first, in which case fn has not run.
func (s *Set) Do(ctx context.Context, key string, fn func() error) error {
	b := s.breaker(key)
	// Waiting callers poll. A quarter of the cool-down keeps the delay after it ends
	// small without waking up needlessly often.
	poll := max(s.settings.Cooldown/4, time.Millisecond)
	for {
		_, err := b.Execute(func() (struct{}, error) { return struct{}{}, fn() })
		if !errors.Is(err, gobreaker.ErrOpenState) && !errors.Is(err, gobreaker.ErrTooManyRequests) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}
