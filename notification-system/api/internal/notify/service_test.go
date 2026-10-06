package notify_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify/notifytest"
)

const appID = "app-1"

// newService returns a Service over a mock, and the recorder on which a test states the
// calls it expects. Any other call fails the test.
func newService(t *testing.T) (*notify.Service, *notifytest.MockRepositoryMockRecorder) {
	t.Helper()
	repo := notifytest.NewMockRepository(gomock.NewController(t))
	return notify.NewService(repo), repo.EXPECT()
}

func TestSendRejectsInvalidRequests(t *testing.T) {
	body := notify.Content{Body: "hello"}
	tests := []struct {
		name string
		req  notify.Request
	}{
		{name: "NoAudience", req: notify.Request{Content: body}},
		{name: "TooManyUsers", req: notify.Request{UserIDs: make([]string, notify.MaxUserIDsPerJob+1), Content: body}},
		{name: "NoBody", req: notify.Request{UserIDs: []string{"ana"}, Content: notify.Content{Title: "hi"}}},
		{name: "UnknownPriority", req: notify.Request{UserIDs: []string{"ana"}, Content: body, Priority: "urgent"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, _ := newService(t)
			_, created, err := svc.Send(t.Context(), appID, test.req)
			if created || !errors.Is(err, audience.ErrInvalid) {
				t.Errorf("Send(%s) = _, %t, %v, want false, ErrInvalid", test.name, created, err)
			}
		})
	}
}

func TestSend(t *testing.T) {
	svc, repo := newService(t)
	req := notify.Request{UserIDs: []string{"ana"}, ListID: "l1", Content: notify.Content{Body: "hello"}}
	// The priority the repository is given has its default filled in.
	stored := req
	stored.Priority = notify.PriorityNormal
	want := notify.Job{ID: "j1", Status: notify.StatusPending, Priority: notify.PriorityNormal}

	repo.List(gomock.Any(), appID, "l1").Return(audience.List{ID: "l1"}, nil)
	repo.Quota(gomock.Any(), appID).Return(int64(9), int64(10), nil)
	repo.CreateJob(gomock.Any(), appID, stored).Return(want, true, nil)

	got, created, err := svc.Send(t.Context(), appID, req)
	if err != nil || !created {
		t.Fatalf("Send(%+v) = _, %t, %v, want true, nil", req, created, err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Send(%+v) returned unexpected diff (-want +got):\n%s", req, diff)
	}
}

func TestSendToUnknownList(t *testing.T) {
	svc, repo := newService(t)
	repo.List(gomock.Any(), appID, "missing").Return(audience.List{}, audience.ErrNotFound)

	req := notify.Request{ListID: "missing", Content: notify.Content{Body: "hello"}}
	if _, _, err := svc.Send(t.Context(), appID, req); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("Send(unknown list) = %v, want ErrNotFound", err)
	}
}

func TestSendOverQuota(t *testing.T) {
	svc, repo := newService(t)
	repo.Quota(gomock.Any(), appID).Return(int64(10), int64(10), nil)
	// No CreateJob call is expected: nothing is stored once the quota is used.

	req := notify.Request{UserIDs: []string{"ana"}, Content: notify.Content{Body: "hello"}}
	if _, _, err := svc.Send(t.Context(), appID, req); !errors.Is(err, notify.ErrQuotaExceeded) {
		t.Errorf("Send(with the quota used) = %v, want ErrQuotaExceeded", err)
	}
}

func TestSendWithUsedIdempotencyKey(t *testing.T) {
	svc, repo := newService(t)
	first := notify.Job{ID: "j1", Status: notify.StatusDispatched}
	repo.Quota(gomock.Any(), appID).Return(int64(0), int64(10), nil)
	repo.CreateJob(gomock.Any(), appID, gomock.Any()).Return(first, false, nil)

	req := notify.Request{UserIDs: []string{"ana"}, Content: notify.Content{Body: "hello"}, IdempotencyKey: "k"}
	got, created, err := svc.Send(t.Context(), appID, req)
	if err != nil || created || got.ID != "j1" {
		t.Errorf("Send(used idempotency key) = %+v, %t, %v, want job j1, false, nil", got, created, err)
	}
}
