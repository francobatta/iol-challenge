package breaker_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/francobatta/iol-challenge/notification-system/worker/internal/breaker"
)

var (
	errDown    = errors.New("provider is down")
	errRefused = errors.New("bad address")
)

const cooldown = 40 * time.Millisecond

func newSet(onChange func(key string, open bool)) *breaker.Set {
	return breaker.NewSet(breaker.Settings{
		Threshold: 3,
		Cooldown:  cooldown,
		IsFailure: func(err error) bool { return errors.Is(err, errDown) },
		OnChange:  onChange,
	})
}

// fail makes n calls under key that return err.
func fail(t *testing.T, s *breaker.Set, key string, n int, err error) {
	t.Helper()
	for range n {
		if got := s.Do(t.Context(), key, func() error { return err }); !errors.Is(got, err) {
			t.Fatalf("Do(%s, a call failing with %q) = %v, want that error", key, err, got)
		}
	}
}

func TestDoWaitsWhileOpenAndResumesAfterCooldown(t *testing.T) {
	var opened, closed atomic.Int32
	s := newSet(func(key string, open bool) {
		if open {
			opened.Add(1)
		} else {
			closed.Add(1)
		}
	})
	fail(t, s, "app-1", 3, errDown)
	if got := opened.Load(); got != 1 {
		t.Fatalf("After 3 consecutive failures the breaker opened %d times, want 1", got)
	}

	start := time.Now()
	ran := false
	if err := s.Do(t.Context(), "app-1", func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("Do(on an open breaker) = %v, ran = %t, want nil, true", err, ran)
	}
	if waited := time.Since(start); waited < cooldown/2 {
		t.Errorf("Do(on an open breaker) ran its call after %v, want it to wait out the %v cool-down", waited, cooldown)
	}
	if got := closed.Load(); got != 1 {
		t.Errorf("After a successful probe the breaker closed %d times, want 1", got)
	}
}

func TestDoGivesUpWhenContextEnds(t *testing.T) {
	s := breaker.NewSet(breaker.Settings{
		Threshold: 1,
		Cooldown:  time.Hour,
		IsFailure: func(error) bool { return true },
	})
	fail(t, s, "app-1", 1, errDown)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	ran := false
	err := s.Do(ctx, "app-1", func() error { ran = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || ran {
		t.Errorf("Do(on an open breaker, with a context that ends) = %v, ran = %t, want DeadlineExceeded, false", err, ran)
	}
}

func TestBreakersAreIndependent(t *testing.T) {
	s := newSet(nil)
	fail(t, s, "app-1", 3, errDown)

	ctx, cancel := context.WithTimeout(t.Context(), cooldown/4)
	defer cancel()
	if err := s.Do(ctx, "app-2", func() error { return nil }); err != nil {
		t.Errorf("Do(app-2) while the breaker of app-1 is open = %v, want nil", err)
	}
}

func TestOnlyCountedFailuresOpenTheBreaker(t *testing.T) {
	opened := false
	s := newSet(func(string, bool) { opened = true })
	fail(t, s, "app-1", 10, errRefused)
	if opened {
		t.Error("10 failures that IsFailure does not count opened the breaker")
	}

	// A success in between resets the run of failures.
	fail(t, s, "app-1", 2, errDown)
	if err := s.Do(t.Context(), "app-1", func() error { return nil }); err != nil {
		t.Fatalf("Do(a successful call) = %v, want nil", err)
	}
	fail(t, s, "app-1", 2, errDown)
	if opened {
		t.Error("2 failures, a success and 2 more failures opened a breaker with a threshold of 3")
	}
}

func TestFailedProbeKeepsTheBreakerOpen(t *testing.T) {
	var changes atomic.Int32
	s := newSet(func(string, bool) { changes.Add(1) })
	fail(t, s, "app-1", 3, errDown)
	fail(t, s, "app-1", 1, errDown) // waits out the cool-down, then fails as the probe

	ctx, cancel := context.WithTimeout(t.Context(), cooldown/4)
	defer cancel()
	if err := s.Do(ctx, "app-1", func() error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Do(right after a failed probe) = %v, want it to wait until DeadlineExceeded", err)
	}
	if got := changes.Load(); got != 1 {
		t.Errorf("OnChange was called %d times for an opening and a failed probe, want 1", got)
	}
}
