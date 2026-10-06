// Package provider sends notifications through the services that deliver them: Twilio
// for SMS, Mailchimp for email, and APNs and FCM for push.
//
// The requests have the shape each service expects but are deliberately minimal: they
// are exercised against the mock in cmd/mockprovider, not against the real services.
//
// A failed send wraps [ErrRetryable] or [ErrPermanent], which is all a caller needs to
// decide what to do with the notification.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/francobatta/iol-challenge/notification-system/commons/message"
	"github.com/francobatta/iol-challenge/notification-system/commons/providers"
)

var (
	// ErrRetryable reports a failure that may not happen again: the provider was
	// unreachable, timed out, was throttling (429) or failed (5xx).
	ErrRetryable = errors.New("temporary provider failure")
	// ErrPermanent reports that the provider refused the notification itself, so
	// sending it again would fail the same way.
	ErrPermanent = errors.New("notification refused by provider")
)

// Timeout is how long a provider gets to answer one request.
const Timeout = 3 * time.Second

// builders holds how to call each provider. It must have an entry for every name in
// the commons module's providers package.
var builders = map[string]func(baseURL string, d message.Delivery) (*http.Request, error){
	providers.Twilio:    twilioRequest,
	providers.Mailchimp: mailchimpRequest,
	providers.APNs:      apnsRequest,
	providers.FCM:       fcmRequest,
}

// A Client sends notifications through one provider. It is safe for concurrent use.
type Client struct {
	name    string
	baseURL string
	http    *http.Client
	build   func(baseURL string, d message.Delivery) (*http.Request, error)
}

// New returns a Client for the named provider, whose API is at baseURL. It returns an
// error if there is no such provider.
func New(name, baseURL string) (*Client, error) {
	build, ok := builders[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q; it must be one of %s", name, strings.Join(providers.Names(), ", "))
	}
	return &Client{
		name:    name,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		// One connection pool per client, sized for a pod's worth of concurrent
		// sends: the default keeps only two idle connections per host and would
		// open a new one for almost every request.
		http: &http.Client{
			Timeout:   Timeout,
			Transport: &http.Transport{MaxIdleConns: 512, MaxIdleConnsPerHost: 512, IdleConnTimeout: 90 * time.Second},
		},
		build: build,
	}, nil
}

// Send delivers one notification. A nil error means the provider accepted it; any
// other wraps ErrRetryable or ErrPermanent.
func (c *Client) Send(ctx context.Context, d message.Delivery) error {
	req, err := c.build(c.baseURL, d)
	if err != nil {
		return fmt.Errorf("%w: building the %s request: %v", ErrPermanent, c.name, err)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("%w: calling %s: %v", ErrRetryable, c.name, err)
	}
	defer resp.Body.Close()
	// Reading the body to the end lets the connection be reused. What it says does
	// not matter here, nor does failing to read it.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("%w: %s answered %d", ErrRetryable, c.name, resp.StatusCode)
	default:
		return fmt.Errorf("%w: %s answered %d", ErrPermanent, c.name, resp.StatusCode)
	}
}

func jsonRequest(url string, body any) (*http.Request, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// twilioRequest sends an SMS the way Twilio's Messages resource takes it: a form.
func twilioRequest(baseURL string, d message.Delivery) (*http.Request, error) {
	form := url.Values{"To": {d.Address}, "Body": {text(d.Content)}}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/twilio", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("I-Twilio-Idempotency-Token", d.MessageID)
	return req, nil
}

// mailchimpRequest sends an email in the shape of Mailchimp Transactional's
// messages/send.
func mailchimpRequest(baseURL string, d message.Delivery) (*http.Request, error) {
	type recipient struct {
		Email string `json:"email"`
	}
	type msg struct {
		To      []recipient `json:"to"`
		Subject string      `json:"subject"`
		Text    string      `json:"text"`
	}
	req, err := jsonRequest(baseURL+"/mailchimp", struct {
		Message msg `json:"message"`
	}{msg{To: []recipient{{Email: d.Address}}, Subject: d.Content.Title, Text: d.Content.Body}})
	if err != nil {
		return nil, err
	}
	req.Header.Set("Idempotency-Key", d.MessageID)
	return req, nil
}

// An alert is the title and body of a push notification, which APNs and FCM both take.
type alert struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
}

// apnsRequest sends a push notification the way APNs takes it: the device token is in
// the path and the payload is under "aps".
func apnsRequest(baseURL string, d message.Delivery) (*http.Request, error) {
	type aps struct {
		Alert alert `json:"alert"`
	}
	req, err := jsonRequest(baseURL+"/apns/"+url.PathEscape(d.Address), struct {
		APS aps `json:"aps"`
	}{aps{Alert: alert(d.Content)}})
	if err != nil {
		return nil, err
	}
	// APNs treats notifications with the same collapse ID as one.
	req.Header.Set("apns-collapse-id", d.MessageID)
	return req, nil
}

// fcmRequest sends a push notification in the shape of FCM's messages:send.
func fcmRequest(baseURL string, d message.Delivery) (*http.Request, error) {
	type msg struct {
		Token        string            `json:"token"`
		Notification alert             `json:"notification"`
		Data         map[string]string `json:"data"`
	}
	// FCM has no idempotency key; the ID travels in the data for the app to use.
	return jsonRequest(baseURL+"/fcm", struct {
		Message msg `json:"message"`
	}{msg{Token: d.Address, Notification: alert(d.Content), Data: map[string]string{"message_id": d.MessageID}}})
}

// text renders content for a channel that has no separate title.
func text(c message.Content) string {
	if c.Title == "" {
		return c.Body
	}
	return c.Title + ": " + c.Body
}
