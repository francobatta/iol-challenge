package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
)

func TestSendNotification(t *testing.T) {
	c, m := startTestAPI(t)
	c.idempotencyKey = "key-1"
	want := notify.Request{
		UserIDs:        []string{"ana", "bob"},
		ListID:         "l1",
		Priority:       notify.PriorityHigh,
		Content:        notify.Content{Title: "hi", Body: "hello"},
		IdempotencyKey: "key-1",
	}
	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	m.jobs.List(gomock.Any(), testAppID, "l1").Return(audience.List{ID: "l1"}, nil)
	m.jobs.Quota(gomock.Any(), testAppID).Return(int64(0), int64(100), nil)
	m.jobs.CreateJob(gomock.Any(), testAppID, want).Return(notify.Job{
		ID: "j1", Status: notify.StatusPending, Priority: notify.PriorityHigh, CreatedAt: created,
	}, nil)

	const body = `{"user_ids": ["ana", "bob"], "list_id": "l1", "priority": "high", "content": {"title": "hi", "body": "hello"}}`
	var got map[string]any
	if status := c.call(t, "POST", "/v1/notifications", body, &got); status != http.StatusAccepted {
		t.Fatalf("POST /v1/notifications = %d, want %d", status, http.StatusAccepted)
	}
	wantBody := map[string]any{
		"job_id":     "j1",
		"status":     "pending",
		"priority":   "high",
		"queued":     0.0,
		"created_at": "2026-10-05T12:00:00Z",
	}
	if diff := cmp.Diff(wantBody, got); diff != "" {
		t.Errorf("POST /v1/notifications returned unexpected diff (-want +got):\n%s", diff)
	}
}

func TestSendNotificationAgainWithSameIdempotencyKey(t *testing.T) {
	c, m := startTestAPI(t)
	c.idempotencyKey = "key-1"
	m.jobs.Quota(gomock.Any(), testAppID).Return(int64(0), int64(100), nil)
	m.jobs.CreateJob(gomock.Any(), testAppID, gomock.Any()).Return(notify.Job{}, audience.ErrConflict)
	m.jobs.JobByIdempotencyKey(gomock.Any(), testAppID, "key-1").Return(notify.Job{ID: "j1", Status: notify.StatusDispatched}, nil)

	const body = `{"user_ids": ["ana"], "content": {"body": "hello"}}`
	var got notify.Job
	if status := c.call(t, "POST", "/v1/notifications", body, &got); status != http.StatusOK || got.ID != "j1" {
		t.Errorf("POST /v1/notifications with a used idempotency key = %d, job %q, want %d, job %q", status, got.ID, http.StatusOK, "j1")
	}
}

func TestNotificationStatus(t *testing.T) {
	ctx := gomock.Any()
	tests := []struct {
		name               string
		method, path, body string
		expect             func(m mocks)
		want               int
		wantCode           string
	}{
		{
			name: "SendWithoutAudience", method: "POST", path: "/v1/notifications",
			body: `{"content": {"body": "hello"}}`,
			want: http.StatusBadRequest, wantCode: "invalid_request",
		},
		{
			name: "SendWithoutBody", method: "POST", path: "/v1/notifications",
			body: `{"user_ids": ["ana"], "content": {"title": "hi"}}`,
			want: http.StatusBadRequest, wantCode: "invalid_request",
		},
		{
			name: "SendWithUnknownPriority", method: "POST", path: "/v1/notifications",
			body: `{"user_ids": ["ana"], "priority": "urgent", "content": {"body": "hello"}}`,
			want: http.StatusBadRequest, wantCode: "invalid_request",
		},
		{
			name: "SendWithUnknownField", method: "POST", path: "/v1/notifications",
			body: `{"user_ids": ["ana"], "content": {"body": "hello"}, "channel": "sms"}`,
			want: http.StatusBadRequest, wantCode: "invalid_request",
		},
		{
			name: "SendToUnknownList", method: "POST", path: "/v1/notifications",
			body: `{"list_id": "l1", "content": {"body": "hello"}}`,
			expect: func(m mocks) {
				m.jobs.List(ctx, testAppID, "l1").Return(audience.List{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound, wantCode: "not_found",
		},
		{
			name: "SendOverQuota", method: "POST", path: "/v1/notifications",
			body: `{"user_ids": ["ana"], "content": {"body": "hello"}}`,
			expect: func(m mocks) {
				m.jobs.Quota(ctx, testAppID).Return(int64(100), int64(100), nil)
			},
			want: http.StatusTooManyRequests, wantCode: "quota_exceeded",
		},
		{
			name: "GetJob", method: "GET", path: "/v1/notifications/j1",
			expect: func(m mocks) {
				m.jobs.Job(ctx, testAppID, "j1").Return(notify.Job{ID: "j1"}, nil)
			},
			want: http.StatusOK,
		},
		{
			name: "GetUnknownJob", method: "GET", path: "/v1/notifications/j1",
			expect: func(m mocks) {
				m.jobs.Job(ctx, testAppID, "j1").Return(notify.Job{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound, wantCode: "not_found",
		},
		{
			name: "ListJobs", method: "GET", path: "/v1/notifications?limit=2&after=j0",
			expect: func(m mocks) {
				m.jobs.Jobs(ctx, testAppID, audience.Page{After: "j0", Limit: 3}).Return([]notify.Job{{ID: "j1"}}, nil)
			},
			want: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, m := startTestAPI(t)
			if test.expect != nil {
				test.expect(m)
			}
			var got struct {
				Error errorDetail `json:"error"`
			}
			status := c.call(t, test.method, test.path, test.body, &got)
			if status != test.want || got.Error.Code != test.wantCode {
				t.Errorf("%s %s with body %q = %d, error code %q, want %d, error code %q",
					test.method, test.path, test.body, status, got.Error.Code, test.want, test.wantCode)
			}
		})
	}
}
