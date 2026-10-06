package consume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/francobatta/iol-challenge/notification-system/commons/message"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/breaker"
	"github.com/francobatta/iol-challenge/notification-system/worker/internal/provider"
)

var (
	errThrottled = fmt.Errorf("%w: 429", provider.ErrRetryable)
	errRefused   = fmt.Errorf("%w: 400", provider.ErrPermanent)
)

// A fakeSender answers every send with err, or with what fn returns if it is set.
type fakeSender struct {
	err error
	fn  func(d message.Delivery) error

	mu   sync.Mutex
	sent []string // message IDs
}

func (s *fakeSender) Send(ctx context.Context, d message.Delivery) error {
	s.mu.Lock()
	s.sent = append(s.sent, d.MessageID)
	s.mu.Unlock()
	if s.fn != nil {
		return s.fn(d)
	}
	return s.err
}

// A fakePublisher records what is published, as one line per message, and fails every
// publish with err if it is set.
type fakePublisher struct {
	err error

	mu        sync.Mutex
	published []string
}

func (p *fakePublisher) record(format string, args ...any) error {
	if p.err != nil {
		return p.err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, fmt.Sprintf(format, args...))
	return nil
}

func (p *fakePublisher) PublishRetry(ctx context.Context, tier int, msg Message) error {
	return p.record("retry tier %d to %s", tier, msg.Queue)
}

func (p *fakePublisher) PublishDead(ctx context.Context, msg Message, reason string) error {
	return p.record("dead: %s", reason)
}

const testQueue = "notify.send.twilio.app-1"

// newTestHandler returns a Handler over fakes. The zero settings give breakers that
// never open.
func newTestHandler(sender Sender, pub Publisher, settings breaker.Settings) *Handler {
	if settings.Threshold == 0 {
		settings = breaker.Settings{Threshold: 100, Cooldown: time.Hour}
	}
	settings.IsFailure = func(err error) bool { return errors.Is(err, provider.ErrRetryable) }
	return NewHandler(sender, pub, breaker.NewSet(settings), NewMetrics(prometheus.NewRegistry(), "twilio"), 4)
}

// counted returns how many deliveries of an app h has counted under an outcome.
func counted(h *Handler, appID, outcome string) float64 {
	var m dto.Metric
	if err := h.metrics.deliveries.WithLabelValues(appID, outcome).Write(&m); err != nil {
		panic(err) // a counter cannot fail to report itself
	}
	return m.GetCounter().GetValue()
}

func testMessage(t *testing.T, appID string, attempt int) Message {
	t.Helper()
	body, err := json.Marshal(message.Delivery{MessageID: "j1:e1", JobID: "j1", AppID: appID, Provider: "twilio", Address: "+1"})
	if err != nil {
		t.Fatalf("Setup: encoding a delivery: %v", err)
	}
	return Message{Queue: testQueue, Body: body, Attempt: attempt}
}

func TestHandle(t *testing.T) {
	tests := []struct {
		name        string
		sendErr     error
		attempt     int
		want        []string // what is published, in order
		wantOutcome string   // what the delivery is counted as
	}{
		{name: "Sent", wantOutcome: "sent"},
		{name: "SentOnARetry", attempt: 3, wantOutcome: "sent"},
		{name: "FirstFailureGoesToTheFirstTier", sendErr: errThrottled, want: []string{"retry tier 0 to " + testQueue}, wantOutcome: "retried"},
		{name: "LaterFailureGoesToALaterTier", sendErr: errThrottled, attempt: 2, want: []string{"retry tier 2 to " + testQueue}, wantOutcome: "retried"},
		{name: "LastRetry", sendErr: errThrottled, attempt: 3, want: []string{"retry tier 3 to " + testQueue}, wantOutcome: "retried"},
		{name: "OutOfRetries", sendErr: errThrottled, attempt: 4, want: []string{"dead: out of retries"}, wantOutcome: "failed"},
		{name: "RefusedIsNotRetried", sendErr: errRefused, want: []string{"dead: refused by the provider"}, wantOutcome: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender, pub := &fakeSender{err: test.sendErr}, &fakePublisher{}
			h := newTestHandler(sender, pub, breaker.Settings{})

			if done := h.Handle(t.Context(), testMessage(t, "app-1", test.attempt)); !done {
				t.Errorf("Handle(attempt %d, send error %v) = false, want true", test.attempt, test.sendErr)
			}
			if diff := cmp.Diff(test.want, pub.published); diff != "" {
				t.Errorf("Handle(attempt %d, send error %v) published unexpected messages (-want +got):\n%s", test.attempt, test.sendErr, diff)
			}
			for _, outcome := range []string{"sent", "retried", "failed"} {
				want := 0.0
				if outcome == test.wantOutcome {
					want = 1
				}
				if got := counted(h, "app-1", outcome); got != want {
					t.Errorf("Handle(attempt %d, send error %v) counted %v deliveries of app-1 as %s, want %v", test.attempt, test.sendErr, got, outcome, want)
				}
			}
		})
	}
}

func TestHandleSetsAsideUndecodableDelivery(t *testing.T) {
	sender, pub := &fakeSender{}, &fakePublisher{}
	h := newTestHandler(sender, pub, breaker.Settings{})

	if done := h.Handle(t.Context(), Message{Queue: testQueue, Body: []byte("not json")}); !done {
		t.Error("Handle(undecodable delivery) = false, want true")
	}
	if diff := cmp.Diff([]string{"dead: undecodable"}, pub.published); diff != "" {
		t.Errorf("Handle(undecodable delivery) published unexpected messages (-want +got):\n%s", diff)
	}
	if len(sender.sent) != 0 {
		t.Errorf("Handle(undecodable delivery) sent %q, want nothing sent", sender.sent)
	}
	if got := counted(h, "", "undecodable"); got != 1 {
		t.Errorf("Handle(undecodable delivery) counted %v deliveries as undecodable, want 1", got)
	}
}

func TestHandleKeepsFailedDeliveryThatCannotBePublished(t *testing.T) {
	down := errors.New("broker is down")
	for _, sendErr := range []error{errThrottled, errRefused} {
		h := newTestHandler(&fakeSender{err: sendErr}, &fakePublisher{err: down}, breaker.Settings{})
		// Acknowledging here would lose the delivery: nothing has replaced it.
		if done := h.Handle(t.Context(), testMessage(t, "app-1", 0)); done {
			t.Errorf("Handle(send error %v, with the broker down) = true, want false", sendErr)
		}
		// It will be handled again, and is counted then.
		if got := counted(h, "app-1", "retried") + counted(h, "app-1", "failed"); got != 0 {
			t.Errorf("Handle(send error %v, with the broker down) counted %v deliveries, want 0", sendErr, got)
		}
	}
}

func TestHandleNeedsNoBrokerForSentDelivery(t *testing.T) {
	h := newTestHandler(&fakeSender{}, &fakePublisher{err: errors.New("broker is down")}, breaker.Settings{})
	if done := h.Handle(t.Context(), testMessage(t, "app-1", 0)); !done {
		t.Error("Handle(sent, with the broker down) = false, want true")
	}
}

func TestHandleDoesNotStartDeliveriesDuringShutdown(t *testing.T) {
	sender, pub := &fakeSender{}, &fakePublisher{}
	h := newTestHandler(sender, pub, breaker.Settings{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if done := h.Handle(ctx, testMessage(t, "app-1", 0)); done {
		t.Error("Handle(with a cancelled context) = true, want false")
	}
	if len(sender.sent) != 0 || len(pub.published) != 0 {
		t.Errorf("Handle(with a cancelled context) sent %q and published %q, want neither", sender.sent, pub.published)
	}
}

func TestHandleFinishesDeliveryCancelledMidSend(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	// The worker is told to stop while the provider is answering.
	sender := &fakeSender{fn: func(message.Delivery) error { cancel(); return nil }}
	pub := &fakePublisher{}
	h := newTestHandler(sender, pub, breaker.Settings{})

	if done := h.Handle(ctx, testMessage(t, "app-1", 0)); !done {
		t.Error("Handle(cancelled during the send) = false, want true")
	}
	if got := counted(h, "app-1", "sent"); got != 1 || len(pub.published) != 0 {
		t.Errorf("Handle(cancelled during the send) counted %v deliveries as sent and published %q, want 1 and nothing", got, pub.published)
	}
}

func TestOpenBreakerHoldsOneAppWithoutSpendingAttempts(t *testing.T) {
	const cooldown = 60 * time.Millisecond
	var (
		mu      sync.Mutex
		healthy bool // whether app-1's provider account has recovered
	)
	sender := &fakeSender{fn: func(d message.Delivery) error {
		mu.Lock()
		defer mu.Unlock()
		if d.AppID == "app-1" && !healthy {
			return errThrottled
		}
		return nil
	}}
	pub := &fakePublisher{}
	h := newTestHandler(sender, pub, breaker.Settings{Threshold: 2, Cooldown: cooldown})

	// Two failures open app-1's breaker. Each is retried, as usual.
	h.Handle(t.Context(), testMessage(t, "app-1", 0))
	h.Handle(t.Context(), testMessage(t, "app-1", 0))

	// Another app on the same provider is not held up.
	start := time.Now()
	h.Handle(t.Context(), testMessage(t, "app-2", 0))
	if took := time.Since(start); took > cooldown/2 {
		t.Errorf("Handle(app-2) took %v while the breaker of app-1 was open, want it not to wait", took)
	}

	// A delivery of app-1 waits for the breaker instead of failing, so it is sent, on
	// its first attempt, once the account has recovered.
	mu.Lock()
	healthy = true
	mu.Unlock()
	start = time.Now()
	h.Handle(t.Context(), testMessage(t, "app-1", 0))
	if waited := time.Since(start); waited < cooldown/2 {
		t.Errorf("Handle(app-1) took %v with its breaker open, want it to wait out the %v cool-down", waited, cooldown)
	}

	want := []string{"retry tier 0 to " + testQueue, "retry tier 0 to " + testQueue}
	if diff := cmp.Diff(want, pub.published); diff != "" {
		t.Errorf("Handle published unexpected messages (-want +got):\n%s", diff)
	}
	if app1, app2 := counted(h, "app-1", "sent"), counted(h, "app-2", "sent"); app1 != 1 || app2 != 1 {
		t.Errorf("Handle counted %v deliveries of app-1 and %v of app-2 as sent, want 1 of each", app1, app2)
	}
}
