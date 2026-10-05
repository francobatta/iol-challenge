package audience

import (
	"context"
	"fmt"
	"slices"
	"unicode/utf8"
)

const (
	// MaxUserIDLen is the longest user ID, in characters, an app may choose.
	MaxUserIDLen = 255
	// MaxMembersPerAdd is the most users a single AddMembers call accepts.
	MaxMembersPerAdd = 1000
)

// A Service applies the audience rules on top of a Store.
type Service struct {
	store Store
}

// NewService returns a Service that keeps its data in store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// CreateApp registers a new app.
func (s *Service) CreateApp(ctx context.Context, name string) (App, error) {
	if name == "" {
		return App{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	return s.store.CreateApp(ctx, name)
}

// RegisterUser makes sure the user exists and reports whether this call created it.
func (s *Service) RegisterUser(ctx context.Context, appID, userID string) (u User, created bool, err error) {
	if n := utf8.RuneCountInString(userID); n == 0 || n > MaxUserIDLen {
		return User{}, false, fmt.Errorf("%w: user_id must be 1 to %d characters", ErrInvalid, MaxUserIDLen)
	}
	return s.store.PutUser(ctx, appID, userID)
}

// User returns one user.
func (s *Service) User(ctx context.Context, appID, userID string) (User, error) {
	return s.store.User(ctx, appID, userID)
}

// Users returns a page of the app's users.
func (s *Service) Users(ctx context.Context, appID string, p Page) ([]User, error) {
	return s.store.Users(ctx, appID, p)
}

// DeleteUser deletes a user together with its endpoints and list memberships.
func (s *Service) DeleteUser(ctx context.Context, appID, userID string) error {
	return s.store.DeleteUser(ctx, appID, userID)
}

// CreateEndpoint adds an endpoint to the user e.UserID. The ID of e is ignored and a
// new one is assigned.
func (s *Service) CreateEndpoint(ctx context.Context, appID string, e Endpoint) (Endpoint, error) {
	if err := validateEndpoint(e); err != nil {
		return Endpoint{}, err
	}
	return s.store.CreateEndpoint(ctx, appID, e)
}

// Endpoint returns one endpoint.
func (s *Service) Endpoint(ctx context.Context, appID, endpointID string) (Endpoint, error) {
	return s.store.Endpoint(ctx, appID, endpointID)
}

// Endpoints returns a page of a user's endpoints.
func (s *Service) Endpoints(ctx context.Context, appID, userID string, p Page) ([]Endpoint, error) {
	// Without this check an unknown user would look like a user with no endpoints.
	if _, err := s.store.User(ctx, appID, userID); err != nil {
		return nil, err
	}
	return s.store.Endpoints(ctx, appID, userID, p)
}

// UpdateEndpoint applies u to an endpoint and returns the result.
func (s *Service) UpdateEndpoint(ctx context.Context, appID, endpointID string, u EndpointUpdate) (Endpoint, error) {
	e, err := s.store.Endpoint(ctx, appID, endpointID)
	if err != nil {
		return Endpoint{}, err
	}
	if u.Address != nil {
		e.Address = *u.Address
	}
	if u.Channel != nil {
		e.Channel = *u.Channel
	}
	if u.Provider != nil {
		e.Provider = *u.Provider
	}
	if err := validateEndpoint(e); err != nil {
		return Endpoint{}, err
	}
	return s.store.UpdateEndpoint(ctx, appID, e)
}

// DeleteEndpoint deletes one endpoint.
func (s *Service) DeleteEndpoint(ctx context.Context, appID, endpointID string) error {
	return s.store.DeleteEndpoint(ctx, appID, endpointID)
}

// CreateList creates a list. The ID of l is ignored and a new one is assigned.
func (s *Service) CreateList(ctx context.Context, appID string, l List) (List, error) {
	if l.Name == "" {
		return List{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	return s.store.CreateList(ctx, appID, l)
}

// List returns one list.
func (s *Service) List(ctx context.Context, appID, listID string) (List, error) {
	return s.store.List(ctx, appID, listID)
}

// Lists returns a page of the app's lists.
func (s *Service) Lists(ctx context.Context, appID string, p Page) ([]List, error) {
	return s.store.Lists(ctx, appID, p)
}

// UpdateList applies u to a list and returns the result.
func (s *Service) UpdateList(ctx context.Context, appID, listID string, u ListUpdate) (List, error) {
	l, err := s.store.List(ctx, appID, listID)
	if err != nil {
		return List{}, err
	}
	if u.Name != nil {
		l.Name = *u.Name
	}
	if u.Description != nil {
		l.Description = *u.Description
	}
	if l.Name == "" {
		return List{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	return s.store.UpdateList(ctx, appID, l)
}

// DeleteList deletes a list and its memberships. The users themselves are kept.
func (s *Service) DeleteList(ctx context.Context, appID, listID string) error {
	return s.store.DeleteList(ctx, appID, listID)
}

// AddMembers adds users to a list. It adds all of them or, if any does not exist, none:
// the ErrNotFound it returns then names the unknown users. Users that are already
// members are left as they are.
func (s *Service) AddMembers(ctx context.Context, appID, listID string, userIDs []string) error {
	if len(userIDs) == 0 || len(userIDs) > MaxMembersPerAdd {
		return fmt.Errorf("%w: user_ids must have 1 to %d items", ErrInvalid, MaxMembersPerAdd)
	}
	if _, err := s.store.List(ctx, appID, listID); err != nil {
		return err
	}
	known, err := s.store.KnownUsers(ctx, appID, userIDs)
	if err != nil {
		return err
	}
	var unknown []string
	for _, id := range userIDs {
		if !slices.Contains(known, id) && !slices.Contains(unknown, id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: users %q", ErrNotFound, unknown)
	}
	return s.store.AddMembers(ctx, appID, listID, userIDs)
}

// RemoveMember takes a user out of a list. It succeeds if the user was not a member.
func (s *Service) RemoveMember(ctx context.Context, appID, listID, userID string) error {
	return s.store.RemoveMember(ctx, appID, listID, userID)
}

// Members returns a page of a list's members.
func (s *Service) Members(ctx context.Context, appID, listID string, p Page) ([]Member, error) {
	// Without this check an unknown list would look like an empty one.
	if _, err := s.store.List(ctx, appID, listID); err != nil {
		return nil, err
	}
	return s.store.Members(ctx, appID, listID, p)
}

func validateEndpoint(e Endpoint) error {
	switch {
	case e.Address == "":
		return fmt.Errorf("%w: address is required", ErrInvalid)
	case e.Provider == "":
		return fmt.Errorf("%w: provider is required", ErrInvalid)
	}
	switch e.Channel {
	case ChannelEmail, ChannelSMS, ChannelPush:
		return nil
	}
	return fmt.Errorf("%w: channel must be one of email, sms, push", ErrInvalid)
}
