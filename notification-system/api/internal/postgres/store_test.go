package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

// These tests need a PostgreSQL database with db/schema.sql loaded, such as the one
// "docker compose up -d db" starts, and are skipped unless DATABASE_URL points to it.

// newTestStore returns a Store on the test database and the ID of a new app, so that
// each test works on data of its own.
func newTestStore(ctx context.Context, t *testing.T) (s *Store, appID string) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("Setup: connecting to DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)
	s = NewStore(pool)
	app, err := s.CreateApp(ctx, t.Name())
	if err != nil {
		t.Fatalf("Setup: CreateApp failed: %v", err)
	}
	return s, app.ID
}

func mustPutUsers(ctx context.Context, t *testing.T, s *Store, appID string, userIDs ...string) {
	t.Helper()
	for _, id := range userIDs {
		if _, _, err := s.PutUser(ctx, appID, id); err != nil {
			t.Fatalf("Setup: PutUser(%q) failed: %v", id, err)
		}
	}
}

func TestUsers(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)

	first, created, err := s.PutUser(ctx, appID, "b")
	if err != nil || !created {
		t.Fatalf("PutUser(new user) = _, %t, %v, want true, nil", created, err)
	}
	again, created, err := s.PutUser(ctx, appID, "b")
	if err != nil || created || again != first {
		t.Errorf("PutUser(existing user) = %+v, %t, %v, want %+v, false, nil", again, created, err, first)
	}
	mustPutUsers(ctx, t, s, appID, "c", "a")

	ignoreTimes := cmpopts.IgnoreFields(audience.User{}, "CreatedAt")
	got, err := s.Users(ctx, appID, audience.Page{After: "a", Limit: 5})
	if err != nil {
		t.Fatalf("Users(after a) failed: %v", err)
	}
	if diff := cmp.Diff([]audience.User{{ID: "b"}, {ID: "c"}}, got, ignoreTimes); diff != "" {
		t.Errorf("Users(after a) returned unexpected diff (-want +got):\n%s", diff)
	}

	known, err := s.KnownUsers(ctx, appID, []string{"a", "nobody", "c"})
	if err != nil {
		t.Fatalf("KnownUsers failed: %v", err)
	}
	sorted := cmpopts.SortSlices(func(a, b string) bool { return a < b })
	if diff := cmp.Diff([]string{"a", "c"}, known, sorted); diff != "" {
		t.Errorf("KnownUsers(a, nobody, c) returned unexpected diff (-want +got):\n%s", diff)
	}

	if err := s.DeleteUser(ctx, appID, "a"); err != nil {
		t.Errorf("DeleteUser(existing user) = %v, want nil", err)
	}
	if err := s.DeleteUser(ctx, appID, "a"); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("DeleteUser(deleted user) = %v, want ErrNotFound", err)
	}
	if _, err := s.User(ctx, appID, "a"); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("User(deleted user) = _, %v, want ErrNotFound", err)
	}
}

func TestEndpoints(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	mustPutUsers(ctx, t, s, appID, "ana")
	email := audience.Endpoint{UserID: "ana", Address: "ana@example.com", Channel: audience.ChannelEmail, Provider: "ses"}

	if _, err := s.CreateEndpoint(ctx, appID, audience.Endpoint{UserID: "nobody", Address: "x", Channel: audience.ChannelSMS, Provider: "p"}); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("CreateEndpoint(unknown user) = _, %v, want ErrNotFound", err)
	}
	created, err := s.CreateEndpoint(ctx, appID, email)
	if err != nil {
		t.Fatalf("CreateEndpoint failed: %v", err)
	}
	if diff := cmp.Diff(email, created, cmpopts.IgnoreFields(audience.Endpoint{}, "ID")); diff != "" || created.ID == "" {
		t.Errorf("CreateEndpoint returned ID %q and unexpected diff (-want +got):\n%s", created.ID, diff)
	}
	if _, err := s.CreateEndpoint(ctx, appID, email); !errors.Is(err, audience.ErrConflict) {
		t.Errorf("CreateEndpoint(duplicate) = _, %v, want ErrConflict", err)
	}

	sms := created
	sms.Address, sms.Channel, sms.Provider = "+5491100000000", audience.ChannelSMS, "twilio"
	updated, err := s.UpdateEndpoint(ctx, appID, sms)
	if err != nil || updated != sms {
		t.Errorf("UpdateEndpoint = %+v, %v, want %+v, nil", updated, err, sms)
	}
	second, err := s.CreateEndpoint(ctx, appID, email)
	if err != nil {
		t.Fatalf("CreateEndpoint(second) failed: %v", err)
	}

	got, err := s.Endpoints(ctx, appID, "ana", audience.Page{Limit: 5})
	if err != nil {
		t.Fatalf("Endpoints failed: %v", err)
	}
	if diff := cmp.Diff([]audience.Endpoint{sms, second}, got); diff != "" {
		t.Errorf("Endpoints returned unexpected diff (-want +got):\n%s", diff)
	}
	got, err = s.Endpoints(ctx, appID, "ana", audience.Page{After: sms.ID, Limit: 5})
	if err != nil {
		t.Fatalf("Endpoints(after the first) failed: %v", err)
	}
	if diff := cmp.Diff([]audience.Endpoint{second}, got); diff != "" {
		t.Errorf("Endpoints(after the first) returned unexpected diff (-want +got):\n%s", diff)
	}

	if _, err := s.Endpoint(ctx, appID, "not-a-uuid"); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("Endpoint(malformed ID) = _, %v, want ErrNotFound", err)
	}
	if err := s.DeleteEndpoint(ctx, appID, second.ID); err != nil {
		t.Errorf("DeleteEndpoint = %v, want nil", err)
	}
	if err := s.DeleteEndpoint(ctx, appID, second.ID); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("DeleteEndpoint(deleted endpoint) = %v, want ErrNotFound", err)
	}

	if err := s.DeleteUser(ctx, appID, "ana"); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
	if _, err := s.Endpoint(ctx, appID, sms.ID); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("Endpoint(of a deleted user) = _, %v, want ErrNotFound", err)
	}
}

func TestLists(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)

	beta, err := s.CreateList(ctx, appID, audience.List{Name: "beta", Description: "early access"})
	if err != nil {
		t.Fatalf("CreateList failed: %v", err)
	}
	if _, err := s.CreateList(ctx, appID, audience.List{Name: "beta"}); !errors.Is(err, audience.ErrConflict) {
		t.Errorf("CreateList(duplicate name) = _, %v, want ErrConflict", err)
	}
	vip, err := s.CreateList(ctx, appID, audience.List{Name: "vip"})
	if err != nil {
		t.Fatalf("CreateList(second) failed: %v", err)
	}

	renamed := beta
	renamed.Name = "vip"
	if _, err := s.UpdateList(ctx, appID, renamed); !errors.Is(err, audience.ErrConflict) {
		t.Errorf("UpdateList(taken name) = _, %v, want ErrConflict", err)
	}
	renamed.Name = "testers"
	if got, err := s.UpdateList(ctx, appID, renamed); err != nil || !cmp.Equal(got, renamed) {
		t.Errorf("UpdateList = %+v, %v, want %+v, nil", got, err, renamed)
	}

	got, err := s.Lists(ctx, appID, audience.Page{Limit: 5})
	if err != nil {
		t.Fatalf("Lists failed: %v", err)
	}
	if diff := cmp.Diff([]audience.List{renamed, vip}, got); diff != "" {
		t.Errorf("Lists returned unexpected diff (-want +got):\n%s", diff)
	}

	if err := s.DeleteList(ctx, appID, vip.ID); err != nil {
		t.Errorf("DeleteList = %v, want nil", err)
	}
	if _, err := s.List(ctx, appID, vip.ID); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("List(deleted list) = _, %v, want ErrNotFound", err)
	}
}

func TestMembers(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	mustPutUsers(ctx, t, s, appID, "ana", "bob", "cleo")
	list, err := s.CreateList(ctx, appID, audience.List{Name: "beta"})
	if err != nil {
		t.Fatalf("Setup: CreateList failed: %v", err)
	}

	memberIDs := func() []string {
		t.Helper()
		members, err := s.Members(ctx, appID, list.ID, audience.Page{Limit: 10})
		if err != nil {
			t.Fatalf("Members failed: %v", err)
		}
		ids := []string{}
		for _, m := range members {
			ids = append(ids, m.UserID)
		}
		return ids
	}

	if err := s.AddMembers(ctx, appID, list.ID, []string{"ana", "nobody"}); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("AddMembers(ana, nobody) = %v, want ErrNotFound", err)
	}
	if diff := cmp.Diff([]string{}, memberIDs()); diff != "" {
		t.Errorf("Members after a rejected AddMembers returned unexpected diff (-want +got):\n%s", diff)
	}

	if err := s.AddMembers(ctx, appID, list.ID, []string{"bob"}); err != nil {
		t.Fatalf("AddMembers(bob) failed: %v", err)
	}
	if err := s.AddMembers(ctx, appID, list.ID, []string{"cleo", "bob", "ana", "ana"}); err != nil {
		t.Errorf("AddMembers(with repeated and existing members) = %v, want nil", err)
	}
	if diff := cmp.Diff([]string{"ana", "bob", "cleo"}, memberIDs()); diff != "" {
		t.Errorf("Members returned unexpected diff (-want +got):\n%s", diff)
	}

	if err := s.RemoveMember(ctx, appID, list.ID, "bob"); err != nil {
		t.Errorf("RemoveMember(bob) = %v, want nil", err)
	}
	if err := s.RemoveMember(ctx, appID, list.ID, "bob"); err != nil {
		t.Errorf("RemoveMember(bob, again) = %v, want nil", err)
	}
	if err := s.DeleteUser(ctx, appID, "cleo"); err != nil {
		t.Fatalf("DeleteUser(cleo) failed: %v", err)
	}
	if diff := cmp.Diff([]string{"ana"}, memberIDs()); diff != "" {
		t.Errorf("Members after removals returned unexpected diff (-want +got):\n%s", diff)
	}
}
