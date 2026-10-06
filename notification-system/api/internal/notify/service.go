package notify

import (
	"context"
	"fmt"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

// MaxUserIDsPerJob is the most users a single request may name. Larger audiences are
// addressed through a list.
const MaxUserIDsPerJob = 1000

// A Service applies the notification rules on top of a Repository.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Send accepts a notification for delivery and reports whether this call created the
// job; it did not if r.IdempotencyKey had been used before. Storing the job is all it
// takes: the fan-out stored with it is what the dispatcher picks up.
//
// It returns ErrQuotaExceeded once the app has queued its daily quota. Usage is counted
// as deliveries are published, so requests accepted shortly before the quota is reached
// can take the app over it.
func (s *Service) Send(ctx context.Context, appID string, r Request) (j Job, created bool, err error) {
	if r.Priority == "" {
		r.Priority = PriorityNormal
	}
	switch {
	case len(r.UserIDs) == 0 && r.ListID == "":
		return Job{}, false, fmt.Errorf("%w: user_ids or list_id is required", audience.ErrInvalid)
	case len(r.UserIDs) > MaxUserIDsPerJob:
		return Job{}, false, fmt.Errorf("%w: user_ids must have at most %d items", audience.ErrInvalid, MaxUserIDsPerJob)
	case r.Content.Body == "":
		return Job{}, false, fmt.Errorf("%w: content.body is required", audience.ErrInvalid)
	case r.Priority != PriorityHigh && r.Priority != PriorityNormal:
		return Job{}, false, fmt.Errorf("%w: priority must be one of high, normal", audience.ErrInvalid)
	}
	if r.ListID != "" {
		if _, err := s.repo.List(ctx, appID, r.ListID); err != nil {
			return Job{}, false, err
		}
	}
	used, limit, err := s.repo.Quota(ctx, appID)
	if err != nil {
		return Job{}, false, err
	}
	if used >= limit {
		return Job{}, false, fmt.Errorf("%w: %d of %d deliveries queued today", ErrQuotaExceeded, used, limit)
	}

	return s.repo.CreateJob(ctx, appID, r)
}

func (s *Service) Job(ctx context.Context, appID, jobID string) (Job, error) {
	return s.repo.Job(ctx, appID, jobID)
}

// Jobs returns a page of the app's jobs, oldest first.
func (s *Service) Jobs(ctx context.Context, appID string, p audience.Page) ([]Job, error) {
	return s.repo.Jobs(ctx, appID, p)
}
