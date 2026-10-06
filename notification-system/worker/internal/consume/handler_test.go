package consume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

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

const testQueue = "notify.send.twilio.app-1"

// newTestHandler returns a Handler over a fake. The zero settings give breakers that
// never open.
func newTestHandler(sender Sender, settings breaker.Settings) *Handler {
	if settings.Threshold == 0 {
		settings = breaker.Settings{Threshold: 100, Cooldown: time.Hour}
	}
	settings.IsFailure = func(err error) bool { return errors.Is(err, provider.ErrRetryable) }
	return NewHandler(sender, breaker.NewSet(settings), NewMetrics(prometheus.NewRegistry(), "twilio"), 4)
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
		want        Verdict
		wantOutcome string // what the delivery is counted as
	}{
		{name: "Sent", want: Sent, wantOutcome: "sent"},
		{name: "SentOnARetry", attempt: 3, want: Sent, wantOutcome: "sent"},
		{name: "FailureIsRetried", sendErr: errThrottled, want: Retry, wantOutcome: "retried"},
		// Giving up after too many failures is the broker's call, not the handler's.
		{name: "LaterFailureIsStillRetried", sendErr: errThrottled, attempt: 9, want: Retry, wantOutcome: "retried"},
		{name: "RefusedIsNotRetried", sendErr: errRefused, want: Dead, wantOutcome: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newTestHandler(&fakeSender{err: test.sendErr}, breaker.Settings{})

			if got := h.Handle(t.Context(), testMessage(t, "app-1", test.attempt)); got != test.want {
				t.Errorf("Handle(attempt %d, send error %v) = %v, want %v", test.attempt, test.sendErr, got, test.want)
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
	sender := &fakeSender{}
	h := newTestHandler(sender, breaker.Settings{})

	if got := h.Handle(t.Context(), Message{Queue: testQueue, Body: []byte("not json")}); got != Dead {
		t.Errorf("Handle(undecodable delivery) = %v, want %v", got, Dead)
	}
	if len(sender.sent) != 0 {
		t.Errorf("Handle(undecodable delivery) sent %q, want nothing sent", sender.sent)
	}
	if got := counted(h, "", "undecodable"); got != 1 {
		t.Errorf("Handle(undecodable delivery) counted %v deliveries as undecodable, want 1", got)
	}
}

func TestHandleDoesNotStartDeliveriesDuringShutdown(t *testing.T) {
	sender := &fakeSender{}
	h := newTestHandler(sender, breaker.Settings{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Not Retry: a delivery that was never attempted must not count as a failed one.
	if got := h.Handle(ctx, testMessage(t, "app-1", 0)); got != NotStarted {
		t.Errorf("Handle(with a cancelled context) = %v, want %v", got, NotStarted)
	}
	if len(sender.sent) != 0 {
		t.Errorf("Handle(with a cancelled context) sent %q, want nothing sent", sender.sent)
	}
}

func TestHandleFinishesDeliveryCancelledMidSend(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	// The worker is told to stop while the provider is answering.
	sender := &fakeSender{fn: func(message.Delivery) error { cancel(); return nil }}
	h := newTestHandler(sender, breaker.Settings{})

	if got := h.Handle(ctx, testMessage(t, "app-1", 0)); got != Sent {
		t.Errorf("Handle(cancelled during the send) = %v, want %v", got, Sent)
	}
	if got := counted(h, "app-1", "sent"); got != 1 {
		t.Errorf("Handle(cancelled during the send) counted %v deliveries as sent, want 1", got)
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
	h := newTestHandler(sender, breaker.Settings{Threshold: 2, Cooldown: cooldown})

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

	if got := counted(h, "app-1", "retried"); got != 2 {
		t.Errorf("Handle counted %v deliveries of app-1 as retried, want the 2 that opened the breaker", got)
	}
	if app1, app2 := counted(h, "app-1", "sent"), counted(h, "app-2", "sent"); app1 != 1 || app2 != 1 {
		t.Errorf("Handle counted %v deliveries of app-1 and %v of app-2 as sent, want 1 of each", app1, app2)
	}
}
