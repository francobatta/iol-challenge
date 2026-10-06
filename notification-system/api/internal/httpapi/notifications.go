package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
)

func (s *server) sendNotification(w http.ResponseWriter, r *http.Request, appID string) error {
	var req struct {
		UserIDs  []string        `json:"user_ids"`
		ListID   string          `json:"list_id"`
		Priority notify.Priority `json:"priority"`
		Content  notify.Content  `json:"content"`
	}
	if err := decode(w, r, &req); err != nil {
		return err
	}
	ctx, span := otel.Tracer("api").Start(r.Context(), "notifications.send")
	defer span.End()
	span.SetAttributes(attribute.String("app_id", appID))

	key := r.Header.Get("Idempotency-Key")
	// Accepted, not created: delivery happens later.
	status := http.StatusAccepted
	job, err := s.notifications.Send(ctx, appID, notify.Request{
		UserIDs:        req.UserIDs,
		ListID:         req.ListID,
		Priority:       req.Priority,
		Content:        req.Content,
		IdempotencyKey: key,
	})
	if errors.Is(err, audience.ErrConflict) {
		// A repeated request gets the job the first one created, as it is now.
		status = http.StatusOK
		job, err = s.notifications.JobByIdempotencyKey(ctx, appID, key)
	}
	if err != nil {
		return err
	}
	span.SetAttributes(attribute.String("job_id", job.ID))
	writeJSON(w, status, job)
	return nil
}

func (s *server) notificationJob(w http.ResponseWriter, r *http.Request, appID string) error {
	job, err := s.notifications.Job(r.Context(), appID, chi.URLParam(r, "job_id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, job)
	return nil
}

func (s *server) notificationJobs(w http.ResponseWriter, r *http.Request, appID string) error {
	return servePage(w, r,
		func(j notify.Job) string { return j.ID },
		func(p audience.Page) ([]notify.Job, error) { return s.notifications.Jobs(r.Context(), appID, p) })
}
