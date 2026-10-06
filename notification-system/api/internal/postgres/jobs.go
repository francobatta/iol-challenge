package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/dispatch"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/postgres/queries"
)

func toJob(row queries.Job) notify.Job {
	return notify.Job{
		ID:        row.JobID.String(),
		Status:    notify.Status(row.Status),
		Priority:  notify.Priority(row.Priority),
		Queued:    row.Queued,
		CreatedAt: row.CreatedAt,
	}
}

func (s *Repository) Quota(ctx context.Context, appID string) (used, limit int64, err error) {
	row, err := s.q.Quota(ctx, toUUID(appID))
	if err != nil {
		return 0, 0, translate(err, "app")
	}
	return row.Used, row.DailyQuota, nil
}

func (s *Repository) CreateJob(ctx context.Context, appID string, r notify.Request) (j notify.Job, created bool, err error) {
	userIDs := r.UserIDs
	if userIDs == nil {
		userIDs = []string{} // the column is NOT NULL
	}
	listID := pgtype.UUID{} // NULL: the job has no list
	if r.ListID != "" {
		listID = toUUID(r.ListID)
	}
	key := pgtype.Text{String: r.IdempotencyKey, Valid: r.IdempotencyKey != ""}

	row, err := s.q.InsertJob(ctx, queries.InsertJobParams{
		AppID:          toUUID(appID),
		Priority:       string(r.Priority),
		UserIds:        userIDs,
		ListID:         listID,
		Title:          r.Content.Title,
		Body:           r.Content.Body,
		IdempotencyKey: key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The insert was skipped because the key is taken.
		first, err := s.q.JobByIdempotencyKey(ctx, queries.JobByIdempotencyKeyParams{AppID: toUUID(appID), IdempotencyKey: key})
		if err != nil {
			return notify.Job{}, false, translate(err, "job")
		}
		return toJob(first), false, nil
	}
	if err != nil {
		return notify.Job{}, false, translate(err, "app")
	}
	return toJob(queries.Job(row)), true, nil
}

func (s *Repository) Job(ctx context.Context, appID, jobID string) (notify.Job, error) {
	row, err := s.q.Job(ctx, queries.JobParams{AppID: toUUID(appID), JobID: toUUID(jobID)})
	if err != nil {
		return notify.Job{}, translate(err, "job")
	}
	return toJob(row), nil
}

func (s *Repository) Jobs(ctx context.Context, appID string, p audience.Page) ([]notify.Job, error) {
	rows, err := s.q.Jobs(ctx, queries.JobsParams{
		AppID:   toUUID(appID),
		After:   afterUUID(p.After),
		MaxRows: int32(p.Limit),
	})
	if err != nil {
		return nil, err
	}
	jobs := make([]notify.Job, len(rows))
	for i, row := range rows {
		jobs[i] = toJob(row)
	}
	return jobs, nil
}

func (s *Repository) ClaimFanout(ctx context.Context, lease time.Duration) (j dispatch.Job, ok bool, err error) {
	row, err := s.q.ClaimFanout(ctx, lease.Seconds())
	if errors.Is(err, pgx.ErrNoRows) {
		return dispatch.Job{}, false, nil
	}
	if err != nil {
		return dispatch.Job{}, false, err
	}
	j = dispatch.Job{
		Ref:      notify.Ref{AppID: row.AppID.String(), JobID: row.JobID.String()},
		Priority: notify.Priority(row.Priority),
		UserIDs:  row.UserIds,
		Content:  notify.Content{Title: row.Title, Body: row.Body},
		Cursor:   row.UserCursor,
	}
	if row.ListID.Valid {
		j.ListID = row.ListID.String()
	}
	return j, true, nil
}

func (s *Repository) FanoutPage(ctx context.Context, j dispatch.Job, limit int) (dispatch.Page, error) {
	listID := pgtype.UUID{} // NULL matches no list
	if j.ListID != "" {
		listID = toUUID(j.ListID)
	}
	rows, err := s.q.FanoutPage(ctx, queries.FanoutPageParams{
		AppID:    toUUID(j.AppID),
		ListID:   listID,
		After:    j.Cursor,
		MaxUsers: int32(limit),
		UserIds:  j.UserIDs,
	})
	if err != nil {
		return dispatch.Page{}, err
	}
	// The rows are ordered by user, so a user's rows are together and the last row is
	// the last user's.
	page := dispatch.Page{Cursor: j.Cursor}
	users := 0
	for _, row := range rows {
		if row.UserID != page.Cursor {
			users++
			page.Cursor = row.UserID
		}
		if !row.EndpointID.Valid {
			continue // the user has no endpoints
		}
		page.Endpoints = append(page.Endpoints, audience.Endpoint{
			ID:       row.EndpointID.String(),
			UserID:   row.UserID,
			Address:  row.Address.String,
			Channel:  audience.Channel(row.Channel.String),
			Provider: row.Provider.String,
		})
	}
	page.Last = users < limit
	return page, nil
}

func (s *Repository) AdvanceFanout(ctx context.Context, j dispatch.Job, cursor string, published int, lease time.Duration) error {
	advanced, err := s.q.AdvanceFanout(ctx, queries.AdvanceFanoutParams{
		AppID:          toUUID(j.AppID),
		JobID:          toUUID(j.JobID),
		PreviousCursor: j.Cursor,
		UserCursor:     cursor,
		Published:      int64(published),
		LeaseSecs:      lease.Seconds(),
	})
	if err != nil {
		return err
	}
	if advanced == 0 {
		return dispatch.ErrClaimLost
	}
	return nil
}

func (s *Repository) FinishFanout(ctx context.Context, j dispatch.Job, published int) error {
	finished, err := s.q.FinishFanout(ctx, queries.FinishFanoutParams{
		AppID:          toUUID(j.AppID),
		JobID:          toUUID(j.JobID),
		PreviousCursor: j.Cursor,
		Published:      int64(published),
	})
	if err != nil {
		return err
	}
	if finished == 0 {
		return dispatch.ErrClaimLost
	}
	return nil
}
