package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/dispatch"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
)

func mustCreateJob(ctx context.Context, t *testing.T, s *Repository, appID string, r notify.Request) notify.Job {
	t.Helper()
	if r.Priority == "" {
		r.Priority = notify.PriorityNormal
	}
	if r.Content.Body == "" {
		r.Content.Body = "hello"
	}
	j, created, err := s.CreateJob(ctx, appID, r)
	if err != nil || !created {
		t.Fatalf("Setup: CreateJob(%+v) = _, %t, %v, want true, nil", r, created, err)
	}
	return j
}

func mustJob(ctx context.Context, t *testing.T, s *Repository, ref notify.Ref) notify.Job {
	t.Helper()
	j, err := s.Job(ctx, ref.AppID, ref.JobID)
	if err != nil {
		t.Fatalf("Job(%+v) failed: %v", ref, err)
	}
	return j
}

// mustClaim claims fan-outs for the lease until it gets one of the app's, and reports
// whether it did. The database is shared, so the fan-outs other tests left behind come
// first; they are claimed for the same lease, which puts them behind the app's.
func mustClaim(ctx context.Context, t *testing.T, s *Repository, appID string, lease time.Duration) (j dispatch.Job, ok bool) {
	t.Helper()
	for range 10_000 {
		j, ok, err := s.ClaimFanout(ctx, lease)
		if err != nil {
			t.Fatalf("ClaimFanout(%v) failed: %v", lease, err)
		}
		if !ok || j.AppID == appID {
			return j, ok
		}
	}
	t.Fatalf("ClaimFanout(%v) did not get to the fan-outs of app %s in 10,000 claims", lease, appID)
	return dispatch.Job{}, false
}

func TestCreateJob(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	req := notify.Request{
		UserIDs:        []string{"ana", "bob"},
		Priority:       notify.PriorityHigh,
		Content:        notify.Content{Title: "hi", Body: "hello"},
		IdempotencyKey: "key-1",
	}

	first, created, err := s.CreateJob(ctx, appID, req)
	if err != nil || !created {
		t.Fatalf("CreateJob = _, %t, %v, want true, nil", created, err)
	}
	want := notify.Job{Status: notify.StatusPending, Priority: notify.PriorityHigh}
	if diff := cmp.Diff(want, first, cmpopts.IgnoreFields(notify.Job{}, "ID", "CreatedAt")); diff != "" || first.ID == "" {
		t.Errorf("CreateJob returned ID %q and unexpected diff (-want +got):\n%s", first.ID, diff)
	}

	again, created, err := s.CreateJob(ctx, appID, req)
	if err != nil || created || again.ID != first.ID {
		t.Errorf("CreateJob(same idempotency key) = job %q, %t, %v, want job %q, false, nil", again.ID, created, err, first.ID)
	}

	// Without a key every request is a new job.
	req.IdempotencyKey = ""
	a, b := mustCreateJob(ctx, t, s, appID, req), mustCreateJob(ctx, t, s, appID, req)
	if a.ID == b.ID {
		t.Errorf("CreateJob(no idempotency key) twice returned the same job %q", a.ID)
	}

	got, err := s.Jobs(ctx, appID, audience.Page{After: first.ID, Limit: 5})
	if err != nil {
		t.Fatalf("Jobs(after the first) failed: %v", err)
	}
	if diff := cmp.Diff([]notify.Job{a, b}, got); diff != "" {
		t.Errorf("Jobs(after the first) returned unexpected diff (-want +got):\n%s", diff)
	}
	if _, err := s.Job(ctx, appID, "not-a-uuid"); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("Job(malformed ID) = _, %v, want ErrNotFound", err)
	}

	// Each job was stored with a fan-out, and the repeated request added none.
	var claimed []string
	for {
		j, ok := mustClaim(ctx, t, s, appID, time.Hour)
		if !ok {
			break
		}
		claimed = append(claimed, j.JobID)
	}
	if diff := cmp.Diff([]string{first.ID, a.ID, b.ID}, claimed); diff != "" {
		t.Errorf("ClaimFanout returned unexpected jobs, oldest first (-want +got):\n%s", diff)
	}
}

func TestClaimFanout(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	list, err := s.CreateList(ctx, appID, audience.List{Name: "beta"})
	if err != nil {
		t.Fatalf("Setup: CreateList failed: %v", err)
	}
	created := mustCreateJob(ctx, t, s, appID, notify.Request{
		UserIDs:  []string{"ana"},
		ListID:   list.ID,
		Priority: notify.PriorityHigh,
		Content:  notify.Content{Title: "hi", Body: "hello"},
	})

	// A lease of nothing leaves the fan-out due, which lets the test claim it again.
	got, ok := mustClaim(ctx, t, s, appID, 0)
	want := dispatch.Job{
		Ref:      notify.Ref{AppID: appID, JobID: created.ID},
		Priority: notify.PriorityHigh,
		UserIDs:  []string{"ana"},
		ListID:   list.ID,
		Content:  notify.Content{Title: "hi", Body: "hello"},
	}
	if diff := cmp.Diff(want, got); diff != "" || !ok {
		t.Fatalf("ClaimFanout of a new job = _, %t with unexpected diff (-want +got):\n%s", ok, diff)
	}

	// A fan-out that is claimed again resumes after the cursor it was left at.
	if err := s.AdvanceFanout(ctx, got, "bob", 1, 0); err != nil {
		t.Fatalf("AdvanceFanout(bob) failed: %v", err)
	}
	if got, ok := mustClaim(ctx, t, s, appID, time.Hour); !ok || got.Cursor != "bob" {
		t.Errorf("ClaimFanout after advancing to bob = cursor %q, %t, want bob, true", got.Cursor, ok)
	}
	if got, ok := mustClaim(ctx, t, s, appID, time.Hour); ok {
		t.Errorf("ClaimFanout while the lease is held returned job %s, want none", got.JobID)
	}
}

func TestFanoutPage(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	mustPutUsers(ctx, t, s, appID, "ana", "bob", "cleo", "dan", "eve")
	list, err := s.CreateList(ctx, appID, audience.List{Name: "beta"})
	if err != nil {
		t.Fatalf("Setup: CreateList failed: %v", err)
	}
	if err := s.AddMembers(ctx, appID, list.ID, []string{"bob", "cleo", "eve"}); err != nil {
		t.Fatalf("Setup: AddMembers failed: %v", err)
	}
	// Everyone but cleo has endpoints, and ana has two.
	endpoint := func(e audience.Endpoint) audience.Endpoint {
		t.Helper()
		created, err := s.CreateEndpoint(ctx, appID, e)
		if err != nil {
			t.Fatalf("Setup: CreateEndpoint(%+v) failed: %v", e, err)
		}
		return created
	}
	ana1 := endpoint(audience.Endpoint{UserID: "ana", Address: "+1", Channel: audience.ChannelSMS, Provider: "twilio"})
	ana2 := endpoint(audience.Endpoint{UserID: "ana", Address: "token", Channel: audience.ChannelPush, Provider: "fcm"})
	bob := endpoint(audience.Endpoint{UserID: "bob", Address: "bob@example.com", Channel: audience.ChannelEmail, Provider: "mailchimp"})
	dan := endpoint(audience.Endpoint{UserID: "dan", Address: "+2", Channel: audience.ChannelSMS, Provider: "twilio"})
	eve := endpoint(audience.Endpoint{UserID: "eve", Address: "ios", Channel: audience.ChannelPush, Provider: "apns"})

	tests := []struct {
		name  string
		job   dispatch.Job
		limit int
		want  dispatch.Page
	}{
		{
			name: "UsersOnly",
			job:  dispatch.Job{UserIDs: []string{"dan", "ana"}}, limit: 10,
			want: dispatch.Page{Endpoints: []audience.Endpoint{ana1, ana2, dan}, Cursor: "dan", Last: true},
		},
		{
			name: "UnknownUsersHaveNoEndpoints",
			job:  dispatch.Job{UserIDs: []string{"ana", "nobody"}}, limit: 10,
			want: dispatch.Page{Endpoints: []audience.Endpoint{ana1, ana2}, Cursor: "nobody", Last: true},
		},
		{
			name: "ListOnly",
			job:  dispatch.Job{ListID: list.ID}, limit: 10,
			want: dispatch.Page{Endpoints: []audience.Endpoint{bob, eve}, Cursor: "eve", Last: true},
		},
		{
			name: "UserAlsoInListAppearsOnce",
			job:  dispatch.Job{UserIDs: []string{"ana", "eve"}, ListID: list.ID}, limit: 10,
			want: dispatch.Page{Endpoints: []audience.Endpoint{ana1, ana2, bob, eve}, Cursor: "eve", Last: true},
		},
		{
			name: "FirstPage",
			job:  dispatch.Job{UserIDs: []string{"ana", "dan"}, ListID: list.ID}, limit: 2,
			want: dispatch.Page{Endpoints: []audience.Endpoint{ana1, ana2, bob}, Cursor: "bob"},
		},
		{
			name: "NextPage",
			job:  dispatch.Job{UserIDs: []string{"ana", "dan"}, ListID: list.ID, Cursor: "bob"}, limit: 2,
			want: dispatch.Page{Endpoints: []audience.Endpoint{dan}, Cursor: "dan"},
		},
		{
			name: "PageOfUsersWithoutEndpoints",
			job:  dispatch.Job{ListID: list.ID, Cursor: "bob"}, limit: 1,
			want: dispatch.Page{Cursor: "cleo"},
		},
		{
			name: "PastTheEnd",
			job:  dispatch.Job{ListID: list.ID, Cursor: "eve"}, limit: 2,
			want: dispatch.Page{Cursor: "eve", Last: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.job.AppID = appID
			got, err := s.FanoutPage(ctx, test.job, test.limit)
			if err != nil {
				t.Fatalf("FanoutPage failed: %v", err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("FanoutPage(after %q, limit %d) returned unexpected diff (-want +got):\n%s", test.job.Cursor, test.limit, diff)
			}
		})
	}
}

func TestJobCounts(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	ref := notify.Ref{AppID: appID, JobID: mustCreateJob(ctx, t, s, appID, notify.Request{UserIDs: []string{"ana"}}).ID}
	fanout := dispatch.Job{Ref: ref}

	if err := s.AdvanceFanout(ctx, fanout, "bob", 3, time.Minute); err != nil {
		t.Fatalf("AdvanceFanout(bob, 3) failed: %v", err)
	}
	// A dispatcher that still believes the cursor is at the start has lost the job, and
	// what it published is not counted again.
	if err := s.AdvanceFanout(ctx, fanout, "bob", 3, time.Minute); !errors.Is(err, dispatch.ErrClaimLost) {
		t.Errorf("AdvanceFanout(from a cursor the fan-out has left) = %v, want ErrClaimLost", err)
	}
	if err := s.FinishFanout(ctx, fanout, 3); !errors.Is(err, dispatch.ErrClaimLost) {
		t.Errorf("FinishFanout(from a cursor the fan-out has left) = %v, want ErrClaimLost", err)
	}
	if got := mustJob(ctx, t, s, ref); got.Status != notify.StatusDispatching || got.Queued != 3 {
		t.Errorf("Job after one page = status %q, queued %d, want dispatching, 3", got.Status, got.Queued)
	}

	fanout.Cursor = "bob"
	if err := s.FinishFanout(ctx, fanout, 2); err != nil {
		t.Fatalf("FinishFanout(2) failed: %v", err)
	}
	if got := mustJob(ctx, t, s, ref); got.Status != notify.StatusDispatched || got.Queued != 5 {
		t.Errorf("Job after FinishFanout = status %q, queued %d, want dispatched, 5", got.Status, got.Queued)
	}
	if used, _, err := s.Quota(ctx, appID); err != nil || used != 5 {
		t.Errorf("Quota after queueing 5 = %d, _, %v, want 5, nil", used, err)
	}
	if got, ok := mustClaim(ctx, t, s, appID, time.Hour); ok {
		t.Errorf("ClaimFanout after FinishFanout returned job %s, want none", got.JobID)
	}
}

func TestFinishFanoutOfEmptyAudience(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	ref := notify.Ref{AppID: appID, JobID: mustCreateJob(ctx, t, s, appID, notify.Request{UserIDs: []string{"nobody"}}).ID}

	if err := s.FinishFanout(ctx, dispatch.Job{Ref: ref}, 0); err != nil {
		t.Fatalf("FinishFanout(0) failed: %v", err)
	}
	if got := mustJob(ctx, t, s, ref); got.Status != notify.StatusDispatched || got.Queued != 0 {
		t.Errorf("Job with nobody to notify = status %q, queued %d after FinishFanout, want dispatched, 0", got.Status, got.Queued)
	}
}
