// Package audience models who an app can notify: its users, the endpoints that reach
// them, and the lists an app groups them into.
//
// Everything belongs to an app, so every operation takes the app's ID and never sees
// another app's data.
//
// Failures that callers are expected to tell apart wrap one of [ErrInvalid],
// [ErrNotFound] or [ErrConflict].
package audience

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalid reports input that breaks a validation rule.
	ErrInvalid = errors.New("invalid")
	// ErrNotFound reports that a user, endpoint or list does not exist in the app.
	ErrNotFound = errors.New("not found")
	// ErrConflict reports that a list name or an endpoint is already taken.
	ErrConflict = errors.New("conflict")
)

// An App is a client of the service. It owns users, endpoints and lists.
type App struct {
	ID        string    `json:"app_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// A User is someone an app can notify. The app chooses the ID.
type User struct {
	ID        string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// A Channel is a kind of notification delivery.
type Channel string

// The channels an endpoint can use.
const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
	ChannelPush  Channel = "push"
)

// An Endpoint is one way to reach a user, such as a phone number or a device token.
type Endpoint struct {
	ID       string  `json:"endpoint_id"`
	UserID   string  `json:"user_id"`
	Address  string  `json:"address"`
	Channel  Channel `json:"channel"`
	Provider string  `json:"provider"` // delivers on the channel, for example "ses" or "fcm"
}

// An EndpointUpdate changes the non-nil fields of an endpoint.
type EndpointUpdate struct {
	Address  *string  `json:"address"`
	Channel  *Channel `json:"channel"`
	Provider *string  `json:"provider"`
}

// A List is a named group of an app's users.
type List struct {
	ID          string    `json:"list_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// A ListUpdate changes the non-nil fields of a list.
type ListUpdate struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// A Member is a user's membership of a list.
type Member struct {
	UserID  string    `json:"user_id"`
	AddedAt time.Time `json:"added_at"`
}

// A Page selects part of a collection, which is always ordered by ID.
type Page struct {
	After string // return items whose ID sorts after this one; empty starts at the beginning
	Limit int    // maximum number of items
}

// Store persists the audience. Implementations must be safe for concurrent use.
//
// Methods that look up or change a single user, endpoint or list return an error
// wrapping [ErrNotFound] when it does not exist in the app. Inputs have already been
// validated by [Service].
//
//go:generate go tool mockgen -destination=audiencetest/mock_store.go -package=audiencetest . Store
type Store interface {
	CreateApp(ctx context.Context, name string) (App, error)

	// PutUser creates the user if it does not exist and reports whether it did so.
	PutUser(ctx context.Context, appID, userID string) (u User, created bool, err error)
	User(ctx context.Context, appID, userID string) (User, error)
	Users(ctx context.Context, appID string, p Page) ([]User, error)
	// KnownUsers returns the subset of userIDs that exist.
	KnownUsers(ctx context.Context, appID string, userIDs []string) ([]string, error)
	// DeleteUser also deletes the user's endpoints and memberships.
	DeleteUser(ctx context.Context, appID, userID string) error

	// CreateEndpoint stores e under a new ID. It returns ErrNotFound if the user does
	// not exist and ErrConflict if the user already has this address on this channel.
	CreateEndpoint(ctx context.Context, appID string, e Endpoint) (Endpoint, error)
	Endpoint(ctx context.Context, appID, endpointID string) (Endpoint, error)
	Endpoints(ctx context.Context, appID, userID string, p Page) ([]Endpoint, error)
	// UpdateEndpoint replaces the address, channel and provider of the endpoint e.ID.
	// It returns ErrConflict under the same rule as CreateEndpoint.
	UpdateEndpoint(ctx context.Context, appID string, e Endpoint) (Endpoint, error)
	DeleteEndpoint(ctx context.Context, appID, endpointID string) error

	// CreateList stores l under a new ID. It returns ErrConflict if the name is taken.
	CreateList(ctx context.Context, appID string, l List) (List, error)
	List(ctx context.Context, appID, listID string) (List, error)
	Lists(ctx context.Context, appID string, p Page) ([]List, error)
	// UpdateList replaces the name and description of the list l.ID. It returns
	// ErrConflict if the name is taken.
	UpdateList(ctx context.Context, appID string, l List) (List, error)
	// DeleteList also deletes the list's memberships.
	DeleteList(ctx context.Context, appID, listID string) error

	// AddMembers adds all the users to the list or none of them. Users that are
	// already members are left as they are. It returns ErrNotFound if the list or any
	// of the users does not exist.
	AddMembers(ctx context.Context, appID, listID string, userIDs []string) error
	// RemoveMember does nothing if the user is not a member.
	RemoveMember(ctx context.Context, appID, listID, userID string) error
	Members(ctx context.Context, appID, listID string, p Page) ([]Member, error)
}
