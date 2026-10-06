package dispatch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/broker"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/dispatch"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/dispatch/dispatchtest"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/commons/message"
)

const lease = time.Minute

var content = notify.Content{Title: "hi", Body: "hello"}

// newMocks returns a repository and a publisher on which a test states the calls it expects.
// Any other call fails the test.
func newMocks(t *testing.T) (*dispatchtest.MockRepository, *dispatchtest.MockPublisher) {
	t.Helper()
	ctrl := gomock.NewController(t)
	return dispatchtest.NewMockRepository(ctrl), dispatchtest.NewMockPublisher(ctrl)
}

func newFanout(repo dispatch.Repository, pub dispatch.Publisher, pageSize int) *dispatch.Fanout {
	return dispatch.NewFanout(repo, pub, dispatch.NewMetrics(prometheus.NewRegistry()), pageSize, lease)
}

// jobAt returns job j1 of app-1 as a dispatcher holds it once fan-out has covered the
// audience up to the user cursor.
func jobAt(cursor string, p notify.Priority) dispatch.Job {
	return dispatch.Job{
		Ref:      notify.Ref{AppID: "app-1", JobID: "j1"},
		Priority: p,
		ListID:   "l1",
		Content:  content,
		Cursor:   cursor,
	}
}

func delivery(endpointID, provider, address string) message.Delivery {
	return message.Delivery{
		MessageID: "j1:" + endpointID,
		JobID:     "j1",
		AppID:     "app-1",
		Provider:  provider,
		Address:   address,
		Content:   message.Content(content),
	}
}

func TestNext(t *testing.T) {
	repo, pub := newMocks(t)
	ctx := gomock.Any()
	high := notify.PriorityHigh

	gomock.InOrder(
		repo.EXPECT().ClaimFanout(ctx, lease).Return(jobAt("", high), nil),

		// First page: two users with three endpoints between them.
		repo.EXPECT().FanoutPage(ctx, jobAt("", high), 2).Return(dispatch.Page{
			Endpoints: []audience.Endpoint{
				{ID: "e1", UserID: "ana", Address: "+1", Channel: audience.ChannelSMS, Provider: "twilio"},
				{ID: "e2", UserID: "ana", Address: "token", Channel: audience.ChannelPush, Provider: "fcm"},
				{ID: "e3", UserID: "bob", Address: "bob@example.com", Channel: audience.ChannelEmail, Provider: "mailchimp"},
			},
			Cursor: "bob",
		}, nil),
		pub.EXPECT().PublishDeliveries(ctx, []message.Delivery{
			delivery("e1", "twilio", "+1"),
			delivery("e2", "fcm", "token"),
			delivery("e3", "mailchimp", "bob@example.com"),
		}, high).Return(nil),
		repo.EXPECT().AdvanceFanout(ctx, jobAt("", high), "bob", 3, lease).Return(nil),

		// Second page: users without endpoints. Nothing is published, but the cursor
		// still moves past them.
		repo.EXPECT().FanoutPage(ctx, jobAt("bob", high), 2).Return(dispatch.Page{Cursor: "dan"}, nil),
		repo.EXPECT().AdvanceFanout(ctx, jobAt("bob", high), "dan", 0, lease).Return(nil),

		// Last page: its deliveries are recorded by finishing.
		repo.EXPECT().FanoutPage(ctx, jobAt("dan", high), 2).Return(dispatch.Page{
			Endpoints: []audience.Endpoint{{ID: "e4", UserID: "eve", Address: "+2", Channel: audience.ChannelSMS, Provider: "twilio"}},
			Cursor:    "eve",
			Last:      true,
		}, nil),
		pub.EXPECT().PublishDeliveries(ctx, []message.Delivery{delivery("e4", "twilio", "+2")}, high).Return(nil),
		repo.EXPECT().FinishFanout(ctx, jobAt("dan", high), 1).Return(nil),
	)

	if claimed, err := newFanout(repo, pub, 2).Next(t.Context()); !claimed || err != nil {
		t.Errorf("Next() = %t, %v, want true, nil", claimed, err)
	}
}

func TestNextWithNothingDue(t *testing.T) {
	repo, pub := newMocks(t)
	repo.EXPECT().ClaimFanout(gomock.Any(), lease).Return(dispatch.Job{}, dispatch.ErrNothingDue)

	if claimed, err := newFanout(repo, pub, 2).Next(t.Context()); claimed || err != nil {
		t.Errorf("Next() with no fan-out due = %t, %v, want false, nil", claimed, err)
	}
}

func TestNextStopsAtFullQueue(t *testing.T) {
	repo, pub := newMocks(t)
	ctx := gomock.Any()
	job := jobAt("", notify.PriorityNormal)

	repo.EXPECT().ClaimFanout(ctx, lease).Return(job, nil)
	repo.EXPECT().FanoutPage(ctx, job, 2).Return(dispatch.Page{
		Endpoints: []audience.Endpoint{{ID: "e1", UserID: "ana", Address: "+1", Channel: audience.ChannelSMS, Provider: "twilio"}},
		Cursor:    "ana",
		Last:      true,
	}, nil)
	pub.EXPECT().PublishDeliveries(ctx, gomock.Any(), notify.PriorityNormal).Return(broker.ErrRejected)
	// Neither AdvanceFanout nor FinishFanout is expected: the cursor must stay before the
	// rejected page so that the next attempt publishes it again.

	claimed, err := newFanout(repo, pub, 2).Next(t.Context())
	if !claimed || !errors.Is(err, broker.ErrRejected) {
		t.Errorf("Next() with a full send queue = %t, %v, want true, ErrRejected", claimed, err)
	}
}

func TestNextReportsRepositoryFailures(t *testing.T) {
	down := errors.New("database is down")
	job := jobAt("", notify.PriorityNormal)
	tests := []struct {
		name        string
		expect      func(repo *dispatchtest.MockRepositoryMockRecorder)
		wantClaimed bool
		wantErr     error
	}{
		{
			name: "Claiming",
			expect: func(repo *dispatchtest.MockRepositoryMockRecorder) {
				repo.ClaimFanout(gomock.Any(), lease).Return(dispatch.Job{}, down)
			},
			wantErr: down,
		},
		{
			name: "ReadingThePage",
			expect: func(repo *dispatchtest.MockRepositoryMockRecorder) {
				repo.ClaimFanout(gomock.Any(), lease).Return(job, nil)
				repo.FanoutPage(gomock.Any(), job, 2).Return(dispatch.Page{}, down)
			},
			wantClaimed: true,
			wantErr:     down,
		},
		{
			name: "LostClaim",
			expect: func(repo *dispatchtest.MockRepositoryMockRecorder) {
				repo.ClaimFanout(gomock.Any(), lease).Return(job, nil)
				repo.FanoutPage(gomock.Any(), job, 2).Return(dispatch.Page{Cursor: "bob"}, nil)
				repo.AdvanceFanout(gomock.Any(), job, "bob", 0, lease).Return(dispatch.ErrClaimLost)
			},
			wantClaimed: true,
			wantErr:     dispatch.ErrClaimLost,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, pub := newMocks(t)
			test.expect(repo.EXPECT())

			claimed, err := newFanout(repo, pub, 2).Next(t.Context())
			if claimed != test.wantClaimed || !errors.Is(err, test.wantErr) {
				t.Errorf("Next() = %t, %v, want %t, %v", claimed, err, test.wantClaimed, test.wantErr)
			}
		})
	}
}

func TestRunFansOutUntilCancelled(t *testing.T) {
	repo, pub := newMocks(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	job := jobAt("", notify.PriorityNormal)

	// One job is due; after it every poll finds nothing. The test ends the run once
	// the job is finished.
	first := repo.EXPECT().ClaimFanout(gomock.Any(), lease).Return(job, nil)
	repo.EXPECT().ClaimFanout(gomock.Any(), lease).Return(dispatch.Job{}, dispatch.ErrNothingDue).After(first).AnyTimes()
	repo.EXPECT().FanoutPage(gomock.Any(), job, 2).Return(dispatch.Page{Last: true}, nil)
	repo.EXPECT().FinishFanout(gomock.Any(), job, 0).DoAndReturn(func(context.Context, dispatch.Job, int) error {
		cancel()
		return nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		newFanout(repo, pub, 2).Run(ctx, 3, time.Millisecond)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not fan the job out and return within 5s of being cancelled")
	}
}
