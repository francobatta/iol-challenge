// Package dispatch is the background work of the API server: turning each accepted job
// into one delivery per endpoint of its audience, for the workers to send. That is the
// job of a [Fanout].
//
// Any number of servers may run at once. They share the work through the fanouts table,
// where each claims the jobs it works on.
package dispatch

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/commons/message"
)

// ErrClaimLost reports that a job's fan-out is no longer where its dispatcher left it:
// the lease ran out and another dispatcher went on with it, or the job is gone.
var ErrClaimLost = errors.New("the fan-out moved on without this dispatcher")

// ErrNothingDue reports that no job is waiting for its fan-out.
var ErrNothingDue = errors.New("no fan-out is due")

// A Job is a job claimed for fan-out: what to send, to whom, and how far it has got.
// What an app is told about a job is notify.Job.
type Job struct {
	notify.Ref
	Priority notify.Priority
	// The job's audience is the users in UserIDs and the members of the list ListID.
	UserIDs []string
	ListID  string
	Content notify.Content
	// Cursor is the last user fan-out has covered; empty if it has not started.
	Cursor string
}

// A Page is a run of consecutive users of a job's audience, as their endpoints.
type Page struct {
	// Endpoints is ordered by user. A user without endpoints adds nothing to it.
	Endpoints []audience.Endpoint
	// Cursor is the last user of the page, to continue after.
	Cursor string
	// Last reports that no user of the audience comes after the page.
	Last bool
}

// Repository is what the dispatcher reads and writes. Implementations must be safe for
// concurrent use.
//
//go:generate go tool mockgen -destination=dispatchtest/mock.go -package=dispatchtest . Repository,Publisher
type Repository interface {
	// ClaimFanout returns the job whose fan-out has been due longest and lets nobody
	// else claim it for the lease. It returns ErrNothingDue if no fan-out is due.
	ClaimFanout(ctx context.Context, lease time.Duration) (Job, error)
	// FanoutPage returns the next limit users of j's audience after j.Cursor, each once.
	// Users the job names that do not exist have no endpoints and so add nothing.
	FanoutPage(ctx context.Context, j Job, limit int) (Page, error)
	// AdvanceFanout records that j's audience is covered up to the user cursor, with
	// that many more deliveries published, and renews the lease. The deliveries count
	// towards the app's usage today. It returns ErrClaimLost, and records nothing, if
	// the fan-out is not at j.Cursor any more.
	AdvanceFanout(ctx context.Context, j Job, cursor string, published int, lease time.Duration) error
	// FinishFanout records the deliveries of the last page and that fan-out is over.
	// It returns ErrClaimLost under the same condition as AdvanceFanout.
	FinishFanout(ctx context.Context, j Job, published int) error
}

// Publisher is what the dispatcher sends to the broker.
type Publisher interface {
	// PublishDeliveries returns broker.ErrRejected if any delivery was refused because
	// its queue is full.
	PublishDeliveries(ctx context.Context, deliveries []message.Delivery, p notify.Priority) error
}

type Metrics struct {
	pages      prometheus.Counter
	deliveries *prometheus.CounterVec
	rejected   prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		pages: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "notify_fanout_pages_total",
			Help: "Pages of users fanned out into deliveries.",
		}),
		// The same labels as the workers' notify_deliveries_total, so that what was
		// queued for an app on a provider can be set against what became of it.
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notify_fanout_deliveries_total",
			Help: "Deliveries published to the send queues, by provider and app.",
		}, []string{"provider", "app_id"}),
		rejected: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "notify_fanout_rejected_total",
			Help: "Fan-out pages put off because a send queue was full.",
		}),
	}
	reg.MustRegister(m.pages, m.deliveries, m.rejected)
	return m
}
