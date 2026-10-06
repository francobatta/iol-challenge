// Package notify accepts notifications from apps and reports how far their dispatch has got.
//
// A notification request becomes a [Job]. Accepting a job only records it; package
// dispatch turns it into one delivery per endpoint and the workers send those. What
// becomes of the deliveries is not recorded on the job: the workers count it in their
// metrics, by app and provider.
//
// Failures that callers are expected to tell apart wrap [audience.ErrInvalid],
// [audience.ErrNotFound] or [ErrQuotaExceeded].
package notify

import (
	"context"
	"errors"
	"time"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

// ErrQuotaExceeded reports that the app has queued all the deliveries it may today.
var ErrQuotaExceeded = errors.New("daily quota exceeded")

// A Priority decides which of an app's deliveries a provider's workers take first.
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
)

// A Status says how far a job has got.
type Status string

// The statuses of a job, in the order it goes through them.
const (
	StatusPending     Status = "pending"     // accepted; fan-out has not started
	StatusDispatching Status = "dispatching" // fan-out in progress; Queued is still growing
	StatusDispatched  Status = "dispatched"  // every delivery has been published; the workers have the rest
)

type Content struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// A Request asks to notify the users in UserIDs and the members of the list ListID. At
// least one of the two is set. A user reached both ways is notified once per endpoint.
type Request struct {
	UserIDs  []string
	ListID   string
	Priority Priority // empty means PriorityNormal
	Content  Content
	// IdempotencyKey, if not empty, makes a repeated request return the first job.
	IdempotencyKey string
}

// A Job is what an app is told about an accepted Request: how far its dispatch has got
// and how many deliveries it came to.
type Job struct {
	ID       string   `json:"job_id"`
	Status   Status   `json:"status"`
	Priority Priority `json:"priority"`
	// Queued counts the deliveries published so far.
	Queued    int64     `json:"queued"`
	CreatedAt time.Time `json:"created_at"`
}

// A Ref names a job of any app.
type Ref struct {
	AppID string `json:"app_id"`
	JobID string `json:"job_id"`
}

// Repository persists jobs. Implementations must be safe for concurrent use.
//
//go:generate go tool mockgen -destination=notifytest/mock.go -package=notifytest . Repository
type Repository interface {
	// List returns ErrNotFound if the list does not exist in the app.
	List(ctx context.Context, appID, listID string) (audience.List, error)
	// Quota returns the deliveries the app has queued today and how many it may queue.
	Quota(ctx context.Context, appID string) (used, limit int64, err error)
	// CreateJob stores a pending job for r and the fan-out it is owed, which is what
	// gets it dispatched. If the app has already used r.IdempotencyKey it returns the
	// job that did, and created is false.
	CreateJob(ctx context.Context, appID string, r Request) (j Job, created bool, err error)
	// Job returns ErrNotFound if the job does not exist in the app.
	Job(ctx context.Context, appID, jobID string) (Job, error)
	// Jobs returns the app's jobs oldest first.
	Jobs(ctx context.Context, appID string, p audience.Page) ([]Job, error)
}
