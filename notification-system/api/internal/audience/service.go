package audience

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/francobatta/iol-challenge/notification-system/commons/providers"
)

const (
	// MaxUserIDLen is the longest user ID, in characters, an app may choose.
	MaxUserIDLen = 255
	// MaxMembersPerAdd is the most users a single AddMembers call accepts.
	MaxMembersPerAdd = 1000
	// ImportBatchSize is how many endpoints Import stores at a time.
	ImportBatchSize = 1000
)

// A Service applies the audience rules on top of a Repository.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateApp(ctx context.Context, name string) (App, error) {
	if name == "" {
		return App{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	return s.repo.CreateApp(ctx, name)
}

// RegisterUser creates the user. It returns ErrConflict if the user already exists;
// User then returns it.
func (s *Service) RegisterUser(ctx context.Context, appID, userID string) (User, error) {
	if err := validateUserID(userID); err != nil {
		return User{}, err
	}
	return s.repo.CreateUser(ctx, appID, userID)
}

func validateUserID(userID string) error {
	if n := utf8.RuneCountInString(userID); n == 0 || n > MaxUserIDLen {
		return fmt.Errorf("%w: user_id must be 1 to %d characters", ErrInvalid, MaxUserIDLen)
	}
	return nil
}

// Import stores endpoints in bulk: it registers the user of each one, gives the user
// the endpoint and, unless listID is empty, adds the user to that list. Users,
// endpoints and memberships that already exist are left as they are, so importing the
// same rows again changes nothing.
//
// rows yields the endpoints, or the error that kept one from being read, which ends
// the import. Errors number the rows from 1. The IDs of the endpoints are ignored and
// new ones are assigned.
//
// Rows are stored ImportBatchSize at a time, so Import is not all-or-nothing: when it
// fails, the batches before the failing one stay, and the result counts them.
func (s *Service) Import(ctx context.Context, appID, listID string, rows iter.Seq2[Endpoint, error]) (ImportResult, error) {
	if listID != "" {
		if _, err := s.repo.List(ctx, appID, listID); err != nil {
			return ImportResult{}, err
		}
	}
	var res ImportResult
	batch := make([]Endpoint, 0, ImportBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		users, endpoints, err := s.repo.ImportEndpoints(ctx, appID, listID, batch)
		if err != nil {
			return err
		}
		res.Rows += len(batch)
		res.Users += users
		res.Endpoints += endpoints
		batch = batch[:0]
		return nil
	}
	for e, err := range rows {
		row := res.Rows + len(batch) + 1
		if err != nil {
			return res, fmt.Errorf("%w: row %d: %v", ErrInvalid, row, err)
		}
		if err := validateUserID(e.UserID); err != nil {
			return res, fmt.Errorf("row %d: %w", row, err)
		}
		if err := validateEndpoint(e); err != nil {
			return res, fmt.Errorf("row %d: %w", row, err)
		}
		batch = append(batch, e)
		if len(batch) == ImportBatchSize {
			if err := flush(); err != nil {
				return res, err
			}
		}
	}
	return res, flush()
}

func (s *Service) User(ctx context.Context, appID, userID string) (User, error) {
	return s.repo.User(ctx, appID, userID)
}

func (s *Service) Users(ctx context.Context, appID string, p Page) ([]User, error) {
	return s.repo.Users(ctx, appID, p)
}

// DeleteUser deletes a user together with its endpoints and list memberships.
func (s *Service) DeleteUser(ctx context.Context, appID, userID string) error {
	return s.repo.DeleteUser(ctx, appID, userID)
}

// CreateEndpoint adds an endpoint to the user e.UserID. The ID of e is ignored and a
// new one is assigned.
func (s *Service) CreateEndpoint(ctx context.Context, appID string, e Endpoint) (Endpoint, error) {
	if err := validateEndpoint(e); err != nil {
		return Endpoint{}, err
	}
	return s.repo.CreateEndpoint(ctx, appID, e)
}

func (s *Service) Endpoint(ctx context.Context, appID, endpointID string) (Endpoint, error) {
	return s.repo.Endpoint(ctx, appID, endpointID)
}

func (s *Service) Endpoints(ctx context.Context, appID, userID string, p Page) ([]Endpoint, error) {
	// Without this check an unknown user would look like a user with no endpoints.
	if _, err := s.repo.User(ctx, appID, userID); err != nil {
		return nil, err
	}
	return s.repo.Endpoints(ctx, appID, userID, p)
}

func (s *Service) UpdateEndpoint(ctx context.Context, appID, endpointID string, u EndpointUpdate) (Endpoint, error) {
	e, err := s.repo.Endpoint(ctx, appID, endpointID)
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
	return s.repo.UpdateEndpoint(ctx, appID, e)
}

func (s *Service) DeleteEndpoint(ctx context.Context, appID, endpointID string) error {
	return s.repo.DeleteEndpoint(ctx, appID, endpointID)
}

// CreateList creates a list. The ID of l is ignored and a new one is assigned.
func (s *Service) CreateList(ctx context.Context, appID string, l List) (List, error) {
	if l.Name == "" {
		return List{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	return s.repo.CreateList(ctx, appID, l)
}

func (s *Service) List(ctx context.Context, appID, listID string) (List, error) {
	return s.repo.List(ctx, appID, listID)
}

func (s *Service) Lists(ctx context.Context, appID string, p Page) ([]List, error) {
	return s.repo.Lists(ctx, appID, p)
}

func (s *Service) UpdateList(ctx context.Context, appID, listID string, u ListUpdate) (List, error) {
	l, err := s.repo.List(ctx, appID, listID)
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
	return s.repo.UpdateList(ctx, appID, l)
}

// DeleteList deletes a list and its memberships. The users themselves are kept.
func (s *Service) DeleteList(ctx context.Context, appID, listID string) error {
	return s.repo.DeleteList(ctx, appID, listID)
}

// AddMembers adds users to a list. It adds all of them or, if any does not exist, none:
// the ErrNotFound it returns then names the unknown users. Users that are already
// members are left as they are.
func (s *Service) AddMembers(ctx context.Context, appID, listID string, userIDs []string) error {
	if len(userIDs) == 0 || len(userIDs) > MaxMembersPerAdd {
		return fmt.Errorf("%w: user_ids must have 1 to %d items", ErrInvalid, MaxMembersPerAdd)
	}
	if _, err := s.repo.List(ctx, appID, listID); err != nil {
		return err
	}
	known, err := s.repo.KnownUsers(ctx, appID, userIDs)
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
	return s.repo.AddMembers(ctx, appID, listID, userIDs)
}

// RemoveMember takes a user out of a list. It succeeds if the user was not a member.
func (s *Service) RemoveMember(ctx context.Context, appID, listID, userID string) error {
	return s.repo.RemoveMember(ctx, appID, listID, userID)
}

func (s *Service) Members(ctx context.Context, appID, listID string, p Page) ([]Member, error) {
	// Without this check an unknown list would look like an empty one.
	if _, err := s.repo.List(ctx, appID, listID); err != nil {
		return nil, err
	}
	return s.repo.Members(ctx, appID, listID, p)
}

// channelProviders lists the providers that can deliver on each channel. Each has a
// worker pool, so an endpoint on any other provider could never be sent to.
var channelProviders = map[Channel][]string{
	ChannelSMS:   {providers.Twilio},
	ChannelEmail: {providers.Mailchimp},
	ChannelPush:  {providers.APNs, providers.FCM},
}

func validateEndpoint(e Endpoint) error {
	switch {
	case e.Address == "":
		return fmt.Errorf("%w: address is required", ErrInvalid)
	case e.Provider == "":
		return fmt.Errorf("%w: provider is required", ErrInvalid)
	}
	allowed, ok := channelProviders[e.Channel]
	if !ok {
		return fmt.Errorf("%w: channel must be one of email, sms, push", ErrInvalid)
	}
	if !slices.Contains(allowed, e.Provider) {
		return fmt.Errorf("%w: provider for channel %s must be one of %s", ErrInvalid, e.Channel, strings.Join(allowed, ", "))
	}
	return nil
}
