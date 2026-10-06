// Package postgres stores the audience and the notification jobs in PostgreSQL.
//
// The SQL lives in db/queries.sql and is compiled by sqlc into the queries package;
// run "sqlc generate" after changing it.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/postgres/queries"
)

// PostgreSQL error codes, from https://www.postgresql.org/docs/current/errcodes-appendix.html.
const (
	foreignKeyViolation = "23503"
	uniqueViolation     = "23505"
)

// A Repository implements the Repository interfaces of the audience, notify and
// dispatch packages on PostgreSQL. Their contracts are documented there.
type Repository struct {
	q *queries.Queries
}

// NewRepository returns a Repository that runs its queries on db, normally a *pgxpool.Pool.
func NewRepository(db queries.DBTX) *Repository {
	return &Repository{q: queries.New(db)}
}

// translate turns a database error into the audience error it stands for. missing
// names what a query with no rows, or a broken foreign key, failed to find.
func translate(err error, missing string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(missing)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case foreignKeyViolation:
			return notFound(missing)
		case uniqueViolation:
			return fmt.Errorf("%w: already exists", audience.ErrConflict)
		}
	}
	return err
}

func notFound(what string) error {
	return fmt.Errorf("%w: %s", audience.ErrNotFound, what)
}

// toUUID converts an ID from its text form. Text that is not a UUID becomes NULL, which
// equals no row, so a malformed ID behaves like one that does not exist.
func toUUID(id string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(id) // on error u stays NULL, which is the result we want
	return u
}

// afterUUID converts a page cursor. The empty cursor becomes the zero UUID, which
// sorts before every real ID.
func afterUUID(after string) pgtype.UUID {
	if after == "" {
		return pgtype.UUID{Valid: true}
	}
	return toUUID(after)
}

func toUser(row queries.User) audience.User {
	return audience.User{ID: row.UserID, CreatedAt: row.CreatedAt}
}

func toEndpoint(row queries.Endpoint) audience.Endpoint {
	return audience.Endpoint{
		ID:       row.EndpointID.String(),
		UserID:   row.UserID,
		Address:  row.Address,
		Channel:  audience.Channel(row.Channel),
		Provider: row.Provider,
	}
}

func toList(row queries.List) audience.List {
	return audience.List{
		ID:          row.ListID.String(),
		Name:        row.Name,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
	}
}

func (s *Repository) CreateApp(ctx context.Context, name string) (audience.App, error) {
	row, err := s.q.CreateApp(ctx, name)
	if err != nil {
		return audience.App{}, err
	}
	return audience.App{ID: row.AppID.String(), Name: row.Name, CreatedAt: row.CreatedAt}, nil
}

func (s *Repository) PutUser(ctx context.Context, appID, userID string) (audience.User, bool, error) {
	inserted, err := s.q.InsertUser(ctx, queries.InsertUserParams{AppID: toUUID(appID), UserID: userID})
	if err != nil {
		return audience.User{}, false, translate(err, "app")
	}
	u, err := s.User(ctx, appID, userID)
	return u, inserted == 1, err
}

func (s *Repository) User(ctx context.Context, appID, userID string) (audience.User, error) {
	row, err := s.q.User(ctx, queries.UserParams{AppID: toUUID(appID), UserID: userID})
	if err != nil {
		return audience.User{}, translate(err, "user")
	}
	return toUser(row), nil
}

func (s *Repository) Users(ctx context.Context, appID string, p audience.Page) ([]audience.User, error) {
	rows, err := s.q.Users(ctx, queries.UsersParams{
		AppID:   toUUID(appID),
		After:   p.After,
		MaxRows: int32(p.Limit),
	})
	if err != nil {
		return nil, err
	}
	users := make([]audience.User, len(rows))
	for i, row := range rows {
		users[i] = toUser(row)
	}
	return users, nil
}

func (s *Repository) KnownUsers(ctx context.Context, appID string, userIDs []string) ([]string, error) {
	return s.q.KnownUsers(ctx, queries.KnownUsersParams{AppID: toUUID(appID), UserIds: userIDs})
}

func (s *Repository) DeleteUser(ctx context.Context, appID, userID string) error {
	deleted, err := s.q.DeleteUser(ctx, queries.DeleteUserParams{AppID: toUUID(appID), UserID: userID})
	if err != nil {
		return err
	}
	if deleted == 0 {
		return notFound("user")
	}
	return nil
}

func (s *Repository) CreateEndpoint(ctx context.Context, appID string, e audience.Endpoint) (audience.Endpoint, error) {
	row, err := s.q.CreateEndpoint(ctx, queries.CreateEndpointParams{
		AppID:    toUUID(appID),
		UserID:   e.UserID,
		Address:  e.Address,
		Channel:  string(e.Channel),
		Provider: e.Provider,
	})
	if err != nil {
		return audience.Endpoint{}, translate(err, "user")
	}
	return toEndpoint(row), nil
}

func (s *Repository) Endpoint(ctx context.Context, appID, endpointID string) (audience.Endpoint, error) {
	row, err := s.q.Endpoint(ctx, queries.EndpointParams{AppID: toUUID(appID), EndpointID: toUUID(endpointID)})
	if err != nil {
		return audience.Endpoint{}, translate(err, "endpoint")
	}
	return toEndpoint(row), nil
}

func (s *Repository) Endpoints(ctx context.Context, appID, userID string, p audience.Page) ([]audience.Endpoint, error) {
	rows, err := s.q.Endpoints(ctx, queries.EndpointsParams{
		AppID:   toUUID(appID),
		UserID:  userID,
		After:   afterUUID(p.After),
		MaxRows: int32(p.Limit),
	})
	if err != nil {
		return nil, err
	}
	endpoints := make([]audience.Endpoint, len(rows))
	for i, row := range rows {
		endpoints[i] = toEndpoint(row)
	}
	return endpoints, nil
}

func (s *Repository) UpdateEndpoint(ctx context.Context, appID string, e audience.Endpoint) (audience.Endpoint, error) {
	row, err := s.q.UpdateEndpoint(ctx, queries.UpdateEndpointParams{
		AppID:      toUUID(appID),
		EndpointID: toUUID(e.ID),
		Address:    e.Address,
		Channel:    string(e.Channel),
		Provider:   e.Provider,
	})
	if err != nil {
		return audience.Endpoint{}, translate(err, "endpoint")
	}
	return toEndpoint(row), nil
}

func (s *Repository) DeleteEndpoint(ctx context.Context, appID, endpointID string) error {
	deleted, err := s.q.DeleteEndpoint(ctx, queries.DeleteEndpointParams{AppID: toUUID(appID), EndpointID: toUUID(endpointID)})
	if err != nil {
		return err
	}
	if deleted == 0 {
		return notFound("endpoint")
	}
	return nil
}

func (s *Repository) CreateList(ctx context.Context, appID string, l audience.List) (audience.List, error) {
	row, err := s.q.CreateList(ctx, queries.CreateListParams{
		AppID:       toUUID(appID),
		Name:        l.Name,
		Description: l.Description,
	})
	if err != nil {
		return audience.List{}, translate(err, "app")
	}
	return toList(row), nil
}

func (s *Repository) List(ctx context.Context, appID, listID string) (audience.List, error) {
	row, err := s.q.List(ctx, queries.ListParams{AppID: toUUID(appID), ListID: toUUID(listID)})
	if err != nil {
		return audience.List{}, translate(err, "list")
	}
	return toList(row), nil
}

func (s *Repository) Lists(ctx context.Context, appID string, p audience.Page) ([]audience.List, error) {
	rows, err := s.q.Lists(ctx, queries.ListsParams{
		AppID:   toUUID(appID),
		After:   afterUUID(p.After),
		MaxRows: int32(p.Limit),
	})
	if err != nil {
		return nil, err
	}
	lists := make([]audience.List, len(rows))
	for i, row := range rows {
		lists[i] = toList(row)
	}
	return lists, nil
}

func (s *Repository) UpdateList(ctx context.Context, appID string, l audience.List) (audience.List, error) {
	row, err := s.q.UpdateList(ctx, queries.UpdateListParams{
		AppID:       toUUID(appID),
		ListID:      toUUID(l.ID),
		Name:        l.Name,
		Description: l.Description,
	})
	if err != nil {
		return audience.List{}, translate(err, "list")
	}
	return toList(row), nil
}

func (s *Repository) DeleteList(ctx context.Context, appID, listID string) error {
	deleted, err := s.q.DeleteList(ctx, queries.DeleteListParams{AppID: toUUID(appID), ListID: toUUID(listID)})
	if err != nil {
		return err
	}
	if deleted == 0 {
		return notFound("list")
	}
	return nil
}

func (s *Repository) AddMembers(ctx context.Context, appID, listID string, userIDs []string) error {
	// A single INSERT, so the foreign keys make it all-or-nothing without a transaction.
	err := s.q.AddMembers(ctx, queries.AddMembersParams{
		AppID:   toUUID(appID),
		ListID:  toUUID(listID),
		UserIds: userIDs,
	})
	return translate(err, "list or user")
}

func (s *Repository) RemoveMember(ctx context.Context, appID, listID, userID string) error {
	return s.q.RemoveMember(ctx, queries.RemoveMemberParams{
		AppID:  toUUID(appID),
		ListID: toUUID(listID),
		UserID: userID,
	})
}

func (s *Repository) Members(ctx context.Context, appID, listID string, p audience.Page) ([]audience.Member, error) {
	rows, err := s.q.Members(ctx, queries.MembersParams{
		AppID:   toUUID(appID),
		ListID:  toUUID(listID),
		After:   p.After,
		MaxRows: int32(p.Limit),
	})
	if err != nil {
		return nil, err
	}
	members := make([]audience.Member, len(rows))
	for i, row := range rows {
		members[i] = audience.Member{UserID: row.UserID, AddedAt: row.AddedAt}
	}
	return members, nil
}
