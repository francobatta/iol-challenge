package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/broker"
	"github.com/francobatta/iol-challenge/notification-system/commons/message"
)

const (
	// DefaultPageSize is how many users Fanout covers between two saves of its position.
	DefaultPageSize = 2000
	// DefaultLease is how long a claimed job is kept from other dispatchers. Every page
	// renews it, so it only has to outlast one page. It is also how long a job waits
	// to be tried again after its fan-out fails or its dispatcher dies.
	DefaultLease = 30 * time.Second
	// DefaultPollInterval is how often an idle dispatcher looks for jobs, and so the
	// longest a job normally waits for its fan-out to start.
	DefaultPollInterval = time.Second
)

// A Fanout turns accepted jobs into deliveries. It finds the jobs in the database,
// where accepting a job leaves a fan-out to be claimed, so nothing has to announce them.
type Fanout struct {
	repo     Repository
	pub      Publisher
	metrics  *Metrics
	pageSize int
	lease    time.Duration
}

// NewFanout returns a Fanout that works through audiences pageSize users at a time,
// holding each job it claims for the lease.
func NewFanout(repo Repository, pub Publisher, metrics *Metrics, pageSize int, lease time.Duration) *Fanout {
	return &Fanout{repo: repo, pub: pub, metrics: metrics, pageSize: pageSize, lease: lease}
}

// Run fans out jobs, up to concurrency of them at a time, until ctx is cancelled. Each
// of its goroutines takes one job after another while there are any, and otherwise
// looks for one every interval.
func (f *Fanout) Run(ctx context.Context, concurrency int, interval time.Duration) {
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() { f.poll(ctx, interval) })
	}
	wg.Wait()
}

func (f *Fanout) poll(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		claimed, err := f.Next(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, broker.ErrRejected):
			f.metrics.rejected.Inc()
			slog.InfoContext(ctx, "A send queue is full; fan-out will resume later", "err", err)
		case err != nil:
			slog.ErrorContext(ctx, "Fan-out failed; it will be retried", "err", err)
		}
		if claimed {
			continue // there may be more
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Next fans out one job that is due, and reports whether it found one.
//
// It publishes the deliveries the job is still owed: one for each endpoint of each user
// in its audience, starting after the job's cursor. Its position is saved after each
// page of users, so a job whose fan-out fails part of the way is continued from there
// once its lease has run out. The page in progress is then published again; the
// deliveries' stable message IDs let providers drop the copies.
//
// The error wraps broker.ErrRejected if a send queue was full.
func (f *Fanout) Next(ctx context.Context) (claimed bool, err error) {
	job, err := f.repo.ClaimFanout(ctx, f.lease)
	if errors.Is(err, ErrNothingDue) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claiming a job to fan out: %w", err)
	}
	ctx, span := otel.Tracer("dispatch").Start(ctx, "fanout")
	span.SetAttributes(attribute.String("app_id", job.AppID), attribute.String("job_id", job.JobID))
	defer span.End()

	if err := f.fanOut(ctx, job); err != nil {
		return true, fmt.Errorf("fanning out job %s of app %s: %w", job.JobID, job.AppID, err)
	}
	return true, nil
}

func (f *Fanout) fanOut(ctx context.Context, job Job) error {
	for {
		page, err := f.repo.FanoutPage(ctx, job, f.pageSize)
		if err != nil {
			return fmt.Errorf("reading the audience after %q: %w", job.Cursor, err)
		}
		deliveries := make([]message.Delivery, len(page.Endpoints))
		for i, e := range page.Endpoints {
			deliveries[i] = message.Delivery{
				MessageID: job.JobID + ":" + e.ID,
				JobID:     job.JobID,
				AppID:     job.AppID,
				Provider:  e.Provider,
				Address:   e.Address,
				Content:   message.Content(job.Content),
			}
		}
		if len(deliveries) > 0 {
			if err := f.pub.PublishDeliveries(ctx, deliveries, job.Priority); err != nil {
				return fmt.Errorf("publishing deliveries: %w", err)
			}
		}
		f.metrics.pages.Inc()
		for _, d := range deliveries {
			f.metrics.deliveries.WithLabelValues(d.Provider, d.AppID).Inc()
		}

		if page.Last {
			if err := f.repo.FinishFanout(ctx, job, len(deliveries)); err != nil {
				return fmt.Errorf("finishing: %w", err)
			}
			return nil
		}
		if err := f.repo.AdvanceFanout(ctx, job, page.Cursor, len(deliveries), f.lease); err != nil {
			return fmt.Errorf("saving the position %q: %w", page.Cursor, err)
		}
		job.Cursor = page.Cursor
	}
}
