// Package audiencetest provides an in-memory audience.Store for tests.
package audiencetest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

// Fake is an audience.Store that keeps everything in memory and follows the same rules
// as the real one: uniqueness, cascading deletes and paging by ID.
type Fake struct {
	mu        sync.Mutex
	lastID    int
	users     map[key]audience.User
	endpoints map[key]audience.Endpoint
	lists     map[key]audience.List
	members   map[memberKey]audience.Member
}

type key struct{ appID, id string }

type memberKey struct{ appID, listID, userID string }

// NewFake returns an empty Fake.
func NewFake() *Fake {
	return &Fake{
		users:     make(map[key]audience.User),
		endpoints: make(map[key]audience.Endpoint),
		lists:     make(map[key]audience.List),
		members:   make(map[memberKey]audience.Member),
	}
}

// newID returns IDs that sort in creation order, as the real store's do.
func (f *Fake) newID() string {
	f.lastID++
	return fmt.Sprintf("%08d", f.lastID)
}

func notFound(what string) error {
	return fmt.Errorf("%w: %s", audience.ErrNotFound, what)
}

// page orders items by ID and returns the part selected by p.
func page[T any](items []T, id func(T) string, p audience.Page) []T {
	items = slices.DeleteFunc(items, func(item T) bool { return id(item) <= p.After })
	slices.SortFunc(items, func(a, b T) int { return strings.Compare(id(a), id(b)) })
	return items[:min(len(items), p.Limit)]
}

func (f *Fake) CreateApp(ctx context.Context, name string) (audience.App, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return audience.App{ID: f.newID(), Name: name, CreatedAt: time.Now()}, nil
}

func (f *Fake) PutUser(ctx context.Context, appID, userID string) (audience.User, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key{appID, userID}
	if u, ok := f.users[k]; ok {
		return u, false, nil
	}
	u := audience.User{ID: userID, CreatedAt: time.Now()}
	f.users[k] = u
	return u, true, nil
}

func (f *Fake) User(ctx context.Context, appID, userID string) (audience.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[key{appID, userID}]
	if !ok {
		return audience.User{}, notFound("user")
	}
	return u, nil
}

func (f *Fake) Users(ctx context.Context, appID string, p audience.Page) ([]audience.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var users []audience.User
	for k, u := range f.users {
		if k.appID == appID {
			users = append(users, u)
		}
	}
	return page(users, func(u audience.User) string { return u.ID }, p), nil
}

func (f *Fake) KnownUsers(ctx context.Context, appID string, userIDs []string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var known []string
	for _, id := range userIDs {
		if _, ok := f.users[key{appID, id}]; ok {
			known = append(known, id)
		}
	}
	return known, nil
}

func (f *Fake) DeleteUser(ctx context.Context, appID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key{appID, userID}
	if _, ok := f.users[k]; !ok {
		return notFound("user")
	}
	delete(f.users, k)
	for ek, e := range f.endpoints {
		if ek.appID == appID && e.UserID == userID {
			delete(f.endpoints, ek)
		}
	}
	for mk := range f.members {
		if mk.appID == appID && mk.userID == userID {
			delete(f.members, mk)
		}
	}
	return nil
}

// endpointTaken reports whether another endpoint of the same user already has e's
// channel and address.
func (f *Fake) endpointTaken(appID string, e audience.Endpoint) bool {
	for k, other := range f.endpoints {
		if k.appID == appID && other.ID != e.ID && other.UserID == e.UserID &&
			other.Channel == e.Channel && other.Address == e.Address {
			return true
		}
	}
	return false
}

func (f *Fake) CreateEndpoint(ctx context.Context, appID string, e audience.Endpoint) (audience.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[key{appID, e.UserID}]; !ok {
		return audience.Endpoint{}, notFound("user")
	}
	e.ID = f.newID()
	if f.endpointTaken(appID, e) {
		return audience.Endpoint{}, audience.ErrConflict
	}
	f.endpoints[key{appID, e.ID}] = e
	return e, nil
}

func (f *Fake) Endpoint(ctx context.Context, appID, endpointID string) (audience.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.endpoints[key{appID, endpointID}]
	if !ok {
		return audience.Endpoint{}, notFound("endpoint")
	}
	return e, nil
}

func (f *Fake) Endpoints(ctx context.Context, appID, userID string, p audience.Page) ([]audience.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var endpoints []audience.Endpoint
	for k, e := range f.endpoints {
		if k.appID == appID && e.UserID == userID {
			endpoints = append(endpoints, e)
		}
	}
	return page(endpoints, func(e audience.Endpoint) string { return e.ID }, p), nil
}

func (f *Fake) UpdateEndpoint(ctx context.Context, appID string, e audience.Endpoint) (audience.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key{appID, e.ID}
	old, ok := f.endpoints[k]
	if !ok {
		return audience.Endpoint{}, notFound("endpoint")
	}
	e.UserID = old.UserID
	if f.endpointTaken(appID, e) {
		return audience.Endpoint{}, audience.ErrConflict
	}
	f.endpoints[k] = e
	return e, nil
}

func (f *Fake) DeleteEndpoint(ctx context.Context, appID, endpointID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key{appID, endpointID}
	if _, ok := f.endpoints[k]; !ok {
		return notFound("endpoint")
	}
	delete(f.endpoints, k)
	return nil
}

// listNameTaken reports whether another list of the app already has l's name.
func (f *Fake) listNameTaken(appID string, l audience.List) bool {
	for k, other := range f.lists {
		if k.appID == appID && other.ID != l.ID && other.Name == l.Name {
			return true
		}
	}
	return false
}

func (f *Fake) CreateList(ctx context.Context, appID string, l audience.List) (audience.List, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l.ID = f.newID()
	l.CreatedAt = time.Now()
	if f.listNameTaken(appID, l) {
		return audience.List{}, audience.ErrConflict
	}
	f.lists[key{appID, l.ID}] = l
	return l, nil
}

func (f *Fake) List(ctx context.Context, appID, listID string) (audience.List, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.lists[key{appID, listID}]
	if !ok {
		return audience.List{}, notFound("list")
	}
	return l, nil
}

func (f *Fake) Lists(ctx context.Context, appID string, p audience.Page) ([]audience.List, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var lists []audience.List
	for k, l := range f.lists {
		if k.appID == appID {
			lists = append(lists, l)
		}
	}
	return page(lists, func(l audience.List) string { return l.ID }, p), nil
}

func (f *Fake) UpdateList(ctx context.Context, appID string, l audience.List) (audience.List, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key{appID, l.ID}
	old, ok := f.lists[k]
	if !ok {
		return audience.List{}, notFound("list")
	}
	l.CreatedAt = old.CreatedAt
	if f.listNameTaken(appID, l) {
		return audience.List{}, audience.ErrConflict
	}
	f.lists[k] = l
	return l, nil
}

func (f *Fake) DeleteList(ctx context.Context, appID, listID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key{appID, listID}
	if _, ok := f.lists[k]; !ok {
		return notFound("list")
	}
	delete(f.lists, k)
	for mk := range f.members {
		if mk.appID == appID && mk.listID == listID {
			delete(f.members, mk)
		}
	}
	return nil
}

func (f *Fake) AddMembers(ctx context.Context, appID, listID string, userIDs []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.lists[key{appID, listID}]; !ok {
		return notFound("list")
	}
	for _, id := range userIDs {
		if _, ok := f.users[key{appID, id}]; !ok {
			return notFound("user")
		}
	}
	for _, id := range userIDs {
		mk := memberKey{appID, listID, id}
		if _, ok := f.members[mk]; !ok {
			f.members[mk] = audience.Member{UserID: id, AddedAt: time.Now()}
		}
	}
	return nil
}

func (f *Fake) RemoveMember(ctx context.Context, appID, listID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.members, memberKey{appID, listID, userID})
	return nil
}

func (f *Fake) Members(ctx context.Context, appID, listID string, p audience.Page) ([]audience.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var members []audience.Member
	for mk, m := range f.members {
		if mk.appID == appID && mk.listID == listID {
			members = append(members, m)
		}
	}
	return page(members, func(m audience.Member) string { return m.UserID }, p), nil
}
