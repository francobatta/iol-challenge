package audience_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience/audiencetest"
)

const appID = "app-1"

func TestRegisterUserValidatesID(t *testing.T) {
	svc := audience.NewService(audiencetest.NewFake())
	tests := []struct {
		name    string
		userID  string
		wantErr error
	}{
		{name: "empty", userID: "", wantErr: audience.ErrInvalid},
		{name: "one character", userID: "a"},
		{name: "longest allowed", userID: strings.Repeat("ñ", audience.MaxUserIDLen)},
		{name: "too long", userID: strings.Repeat("a", audience.MaxUserIDLen+1), wantErr: audience.ErrInvalid},
	}
	for _, test := range tests {
		_, _, err := svc.RegisterUser(t.Context(), appID, test.userID)
		if !errors.Is(err, test.wantErr) {
			t.Errorf("RegisterUser(%s ID) = %v, want error %v", test.name, err, test.wantErr)
		}
	}
}

func TestAddMembers(t *testing.T) {
	ctx := t.Context()
	svc := audience.NewService(audiencetest.NewFake())
	list, err := svc.CreateList(ctx, appID, audience.List{Name: "beta"})
	if err != nil {
		t.Fatalf("Setup: CreateList failed: %v", err)
	}
	if _, _, err := svc.RegisterUser(ctx, appID, "ana"); err != nil {
		t.Fatalf("Setup: RegisterUser failed: %v", err)
	}

	tests := []struct {
		name    string
		listID  string
		userIDs []string
		wantErr error
	}{
		{name: "no users", listID: list.ID, userIDs: nil, wantErr: audience.ErrInvalid},
		{name: "too many users", listID: list.ID, userIDs: make([]string, audience.MaxMembersPerAdd+1), wantErr: audience.ErrInvalid},
		{name: "unknown list", listID: "missing", userIDs: []string{"ana"}, wantErr: audience.ErrNotFound},
		{name: "unknown user", listID: list.ID, userIDs: []string{"ana", "bob"}, wantErr: audience.ErrNotFound},
		{name: "known user", listID: list.ID, userIDs: []string{"ana"}},
		{name: "already a member", listID: list.ID, userIDs: []string{"ana", "ana"}},
	}
	for _, test := range tests {
		err := svc.AddMembers(ctx, appID, test.listID, test.userIDs)
		if !errors.Is(err, test.wantErr) {
			t.Errorf("AddMembers(%s) = %v, want error %v", test.name, err, test.wantErr)
		}
	}
}

func TestAddMembersNamesUnknownUsers(t *testing.T) {
	ctx := t.Context()
	svc := audience.NewService(audiencetest.NewFake())
	list, err := svc.CreateList(ctx, appID, audience.List{Name: "beta"})
	if err != nil {
		t.Fatalf("Setup: CreateList failed: %v", err)
	}
	if _, _, err := svc.RegisterUser(ctx, appID, "ana"); err != nil {
		t.Fatalf("Setup: RegisterUser failed: %v", err)
	}

	err = svc.AddMembers(ctx, appID, list.ID, []string{"bob", "ana", "cleo", "bob"})
	if err == nil {
		t.Fatal("AddMembers with unknown users bob and cleo = nil, want an error")
	}
	for _, id := range []string{"bob", "cleo"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("AddMembers error %q does not name the unknown user %q", err, id)
		}
	}
	if strings.Contains(err.Error(), "ana") {
		t.Errorf("AddMembers error %q names the known user %q", err, "ana")
	}
}
