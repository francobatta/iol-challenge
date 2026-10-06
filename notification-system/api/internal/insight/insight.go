// Package insight reports what the metrics say about an app's deliveries and about the
// system that makes them.
//
// The services count what they do in Prometheus (the README lists the metrics). This
// package asks Prometheus a fixed set of questions and answers with one [Snapshot], so
// that a dashboard needs a single request and never talks to Prometheus, which would
// show it every app's series.
//
// Failures that callers are expected to tell apart wrap [audience.ErrInvalid] or
// [ErrUnavailable].
package insight

import (
	"context"
	"errors"
	"time"
)

// ErrUnavailable reports that the metrics cannot be read: no Prometheus is configured,
// or it did not answer.
var ErrUnavailable = errors.New("metrics unavailable")

// A Sample is the value of one series at one moment.
type Sample struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
}

// A Point is a moment, in Unix seconds, and the value of a series then.
type Point [2]float64

// A Series is how one value changed over the window of a snapshot.
type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}

// Querier evaluates PromQL. Implementations must be safe for concurrent use, and must
// leave out values that are not finite numbers, which JSON cannot carry.
//
//go:generate go tool mockgen -destination=insighttest/mock.go -package=insighttest . Querier
type Querier interface {
	// Instant returns the value of each series of query at the given moment.
	Instant(ctx context.Context, query string, at time.Time) ([]Sample, error)
	// Range returns the values of each series of query from start to end, one every step.
	Range(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error)
}

// A Snapshot is everything a dashboard shows for one app over one window of time.
type Snapshot struct {
	At            time.Time `json:"at"`
	WindowSeconds int       `json:"window_seconds"`
	StepSeconds   int       `json:"step_seconds"`
	App           App       `json:"app"`
	System        System    `json:"system"`
}

// App holds the metrics of the app the snapshot was taken for.
type App struct {
	// Providers has one entry per provider, in the order of providers.Names.
	Providers []ProviderDeliveries `json:"providers"`
	// QueuedRate is deliveries queued per second, by provider.
	QueuedRate []Series `json:"queued_rate"`
	// OutcomeRate is deliveries handled per second, by outcome.
	OutcomeRate []Series `json:"outcome_rate"`
	// Backlog is deliveries in the app's send queues, by provider.
	Backlog []Series `json:"backlog"`
}

// ProviderDeliveries says what became of an app's deliveries on one provider.
type ProviderDeliveries struct {
	Provider string `json:"provider"`

	// Counted over the window.
	Queued      float64 `json:"queued"`
	Sent        float64 `json:"sent"`
	Retried     float64 `json:"retried"`
	Failed      float64 `json:"failed"`
	Undecodable float64 `json:"undecodable"`

	// As of now.
	InTransit   float64 `json:"in_transit"` // queued, ever, but neither sent nor failed
	Ready       float64 `json:"ready"`      // waiting for a worker
	Unacked     float64 `json:"unacked"`    // held by a worker
	BreakerOpen bool    `json:"breaker_open"`
}

// System holds the metrics that are not about any one app.
type System struct {
	Targets []Target     `json:"targets"`
	Pools   []WorkerPool `json:"pools"`
	Queues  []Sample     `json:"queues"` // depth of the retry tiers and the dead-letter queue, by queue

	// Latency of requests to the providers, in seconds, by provider.
	LatencyP50 []Series `json:"latency_p50"`
	LatencyP95 []Series `json:"latency_p95"`
	LatencyP99 []Series `json:"latency_p99"`

	SendsInFlight      []Series `json:"sends_in_flight"`      // by provider
	FanoutPagesRate    []Series `json:"fanout_pages_rate"`    // pages of users per second
	FanoutRejectedRate []Series `json:"fanout_rejected_rate"` // pages put off per second
}

// A Target is a process Prometheus scrapes.
type Target struct {
	Job         string  `json:"job"`
	Instance    string  `json:"instance"`
	Up          bool    `json:"up"`
	Goroutines  float64 `json:"goroutines"`
	MemoryBytes float64 `json:"memory_bytes"`
	CPU         float64 `json:"cpu"` // cores in use
}

// A WorkerPool is the workers of one provider, taken together.
type WorkerPool struct {
	Provider         string  `json:"provider"`
	Workers          float64 `json:"workers"`
	SendsInFlight    float64 `json:"sends_in_flight"`
	QueuesSubscribed float64 `json:"queues_subscribed"` // the most any one worker consumes
}
