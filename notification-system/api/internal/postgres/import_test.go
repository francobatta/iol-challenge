package postgres

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

func TestImportEndpoints(t *testing.T) {
	ctx := t.Context()
	s, appID := newTestStore(ctx, t)
	mustCreateUsers(ctx, t, s, appID, "ana")
	list, err := s.CreateList(ctx, appID, audience.List{Name: "beta"})
	if err != nil {
		t.Fatalf("Setup: CreateList failed: %v", err)
	}
	batch := []audience.Endpoint{
		{UserID: "ana", Address: "ana@example.com", Channel: audience.ChannelEmail, Provider: "mailchimp"},
		{UserID: "bob", Address: "+5491100000000", Channel: audience.ChannelSMS, Provider: "twilio"},
		{UserID: "bob", Address: "android-token", Channel: audience.ChannelPush, Provider: "fcm"},
		{UserID: "bob", Address: "android-token", Channel: audience.ChannelPush, Provider: "fcm"}, // twice in one batch
	}

	// ana exists already, and the last endpoint is listed twice.
	users, endpoints, err := s.ImportEndpoints(ctx, appID, list.ID, batch)
	if err != nil || users != 1 || endpoints != 3 {
		t.Fatalf("ImportEndpoints = %d, %d, %v, want 1, 3, nil", users, endpoints, err)
	}
	users, endpoints, err = s.ImportEndpoints(ctx, appID, list.ID, batch)
	if err != nil || users != 0 || endpoints != 0 {
		t.Errorf("ImportEndpoints(same batch again) = %d, %d, %v, want 0, 0, nil", users, endpoints, err)
	}

	got, err := s.Endpoints(ctx, appID, "bob", audience.Page{Limit: 5})
	if err != nil {
		t.Fatalf("Endpoints(bob) failed: %v", err)
	}
	ignoreIDs := cmpopts.IgnoreFields(audience.Endpoint{}, "ID")
	byAddress := cmpopts.SortSlices(func(a, b audience.Endpoint) bool { return a.Address < b.Address })
	if diff := cmp.Diff(batch[1:3], got, ignoreIDs, byAddress); diff != "" {
		t.Errorf("Endpoints(bob) after the import returned unexpected diff (-want +got):\n%s", diff)
	}
	members, err := s.Members(ctx, appID, list.ID, audience.Page{Limit: 5})
	if err != nil {
		t.Fatalf("Members failed: %v", err)
	}
	if len(members) != 2 || members[0].UserID != "ana" || members[1].UserID != "bob" {
		t.Errorf("Members after the import = %+v, want ana and bob", members)
	}

	// Without a list the users and endpoints are stored and nothing else.
	cleo := []audience.Endpoint{{UserID: "cleo", Address: "ios-token", Channel: audience.ChannelPush, Provider: "apns"}}
	if users, endpoints, err := s.ImportEndpoints(ctx, appID, "", cleo); err != nil || users != 1 || endpoints != 1 {
		t.Errorf("ImportEndpoints(no list) = %d, %d, %v, want 1, 1, nil", users, endpoints, err)
	}

	// An unknown list fails the whole batch: dan is not registered.
	dan := []audience.Endpoint{{UserID: "dan", Address: "ios-token", Channel: audience.ChannelPush, Provider: "apns"}}
	const unknownList = "00000000-0000-7000-8000-000000000000"
	if _, _, err := s.ImportEndpoints(ctx, appID, unknownList, dan); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("ImportEndpoints(unknown list) = _, _, %v, want ErrNotFound", err)
	}
	if _, err := s.User(ctx, appID, "dan"); !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("User(dan) after a failed import = _, %v, want ErrNotFound", err)
	}
}
