package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/francobatta/iol-challenge/notification-system/commons/message"
	"github.com/francobatta/iol-challenge/notification-system/commons/providers"
)

var testDelivery = message.Delivery{
	MessageID: "j1:e1",
	JobID:     "j1",
	AppID:     "app-1",
	Address:   "the-address",
	Content:   message.Content{Title: "Hi", Body: "Hello there"},
}

// newClient returns a Client for the named provider that talks to handler.
func newClient(t *testing.T, name string, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(name, srv.URL)
	if err != nil {
		t.Fatalf("Setup: New(%q) failed: %v", name, err)
	}
	return c
}

func TestSendRequest(t *testing.T) {
	tests := []struct {
		provider     string
		wantPath     string
		wantIDHeader string // the header that carries the message ID, if any
		wantInBody   []string
	}{
		{provider: "twilio", wantPath: "/twilio", wantIDHeader: "I-Twilio-Idempotency-Token", wantInBody: []string{"To=the-address", "Body=Hi%3A+Hello+there"}},
		{provider: "mailchimp", wantPath: "/mailchimp", wantIDHeader: "Idempotency-Key", wantInBody: []string{`"email":"the-address"`, `"subject":"Hi"`, `"text":"Hello there"`}},
		{provider: "apns", wantPath: "/apns/the-address", wantIDHeader: "apns-collapse-id", wantInBody: []string{`"aps":{"alert":{"title":"Hi","body":"Hello there"}}`}},
		{provider: "fcm", wantPath: "/fcm", wantInBody: []string{`"token":"the-address"`, `"message_id":"j1:e1"`, `"body":"Hello there"`}},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			var (
				gotPath, gotID string
				gotBody        []byte
			)
			c := newClient(t, test.provider, func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.Method + " " + r.URL.Path
				if test.wantIDHeader != "" {
					gotID = r.Header.Get(test.wantIDHeader)
				}
				gotBody, _ = io.ReadAll(r.Body) // an unreadable body shows up as a missing substring below
				w.WriteHeader(http.StatusAccepted)
			})

			if err := c.Send(t.Context(), testDelivery); err != nil {
				t.Fatalf("Send = %v, want nil", err)
			}
			if want := "POST " + test.wantPath; gotPath != want {
				t.Errorf("Send requested %q, want %q", gotPath, want)
			}
			if test.wantIDHeader != "" && gotID != "j1:e1" {
				t.Errorf("Send set header %s to %q, want the message ID %q", test.wantIDHeader, gotID, "j1:e1")
			}
			for _, want := range test.wantInBody {
				if !strings.Contains(string(gotBody), want) {
					t.Errorf("Send posted %q, want it to contain %q", gotBody, want)
				}
			}
		})
	}
}

func TestSendClassifiesResponses(t *testing.T) {
	tests := []struct {
		status  int
		wantErr error
	}{
		{status: http.StatusOK},
		{status: http.StatusCreated},
		{status: http.StatusTooManyRequests, wantErr: ErrRetryable},
		{status: http.StatusInternalServerError, wantErr: ErrRetryable},
		{status: http.StatusServiceUnavailable, wantErr: ErrRetryable},
		{status: http.StatusBadRequest, wantErr: ErrPermanent},
		{status: http.StatusUnauthorized, wantErr: ErrPermanent},
		{status: http.StatusGone, wantErr: ErrPermanent},
	}
	for _, test := range tests {
		c := newClient(t, "fcm", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(test.status)
		})
		if err := c.Send(t.Context(), testDelivery); !errors.Is(err, test.wantErr) {
			t.Errorf("Send with the provider answering %d = %v, want %v", test.status, err, test.wantErr)
		}
	}
}

func TestSendTreatsTimeoutAsRetryable(t *testing.T) {
	release := make(chan struct{})
	c := newClient(t, "twilio", func(w http.ResponseWriter, r *http.Request) {
		<-release // answer only once the client has given up
	})
	t.Cleanup(func() { close(release) }) // runs before the server is closed
	c.http.Timeout = 20 * time.Millisecond

	if err := c.Send(t.Context(), testDelivery); !errors.Is(err, ErrRetryable) {
		t.Errorf("Send with the provider not answering = %v, want ErrRetryable", err)
	}
}

func TestSendToUnreachableProviderIsRetryable(t *testing.T) {
	c := newClient(t, "twilio", func(http.ResponseWriter, *http.Request) {})
	c.baseURL = "http://127.0.0.1:1" // nothing listens on port 1

	if err := c.Send(context.Background(), testDelivery); !errors.Is(err, ErrRetryable) {
		t.Errorf("Send to an unreachable provider = %v, want ErrRetryable", err)
	}
}

func TestNewRejectsUnknownProvider(t *testing.T) {
	if _, err := New("ses", "http://localhost"); err == nil {
		t.Error(`New("ses") = nil error, want an error`)
	}
}

func TestEveryProviderHasAClient(t *testing.T) {
	for _, name := range providers.Names() {
		if _, err := New(name, "http://provider.example"); err != nil {
			t.Errorf("New(%q) failed: %v", name, err)
		}
	}
}
