package audience_test

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience/audiencetest"
)

const appID = "app-1"

// newService returns a Service over a mock store, and the recorder on which a test
// states the store calls it expects. Any other store call fails the test.
func newService(t *testing.T) (*audience.Service, *audiencetest.MockStoreMockRecorder) {
	t.Helper()
	store := audiencetest.NewMockStore(gomock.NewController(t))
	return audience.NewService(store), store.EXPECT()
}

func TestRegisterUserValidatesID(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		wantErr error
	}{
		{name: "Empty", userID: "", wantErr: audience.ErrInvalid},
		{name: "OneCharacter", userID: "a"},
		{name: "LongestAllowed", userID: strings.Repeat("ñ", audience.MaxUserIDLen)},
		{name: "TooLong", userID: strings.Repeat("a", audience.MaxUserIDLen+1), wantErr: audience.ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, store := newService(t)
			if test.wantErr == nil {
				store.PutUser(gomock.Any(), appID, test.userID).Return(audience.User{ID: test.userID}, true, nil)
			}
			_, _, err := svc.RegisterUser(t.Context(), appID, test.userID)
			if !errors.Is(err, test.wantErr) {
				t.Errorf("RegisterUser(%q) = %v, want error %v", test.userID, err, test.wantErr)
			}
		})
	}
}

func TestAddMembersRejectsBadBatchSizes(t *testing.T) {
	for _, userIDs := range [][]string{nil, make([]string, audience.MaxMembersPerAdd+1)} {
		svc, _ := newService(t)
		err := svc.AddMembers(t.Context(), appID, "list-1", userIDs)
		if !errors.Is(err, audience.ErrInvalid) {
			t.Errorf("AddMembers(%d users) = %v, want ErrInvalid", len(userIDs), err)
		}
	}
}

func TestAddMembers(t *testing.T) {
	svc, store := newService(t)
	userIDs := []string{"ana", "bob", "ana"}
	store.List(gomock.Any(), appID, "list-1").Return(audience.List{ID: "list-1"}, nil)
	store.KnownUsers(gomock.Any(), appID, userIDs).Return([]string{"bob", "ana"}, nil)
	store.AddMembers(gomock.Any(), appID, "list-1", userIDs).Return(nil)

	if err := svc.AddMembers(t.Context(), appID, "list-1", userIDs); err != nil {
		t.Errorf("AddMembers(%q) = %v, want nil", userIDs, err)
	}
}

func TestAddMembersToUnknownList(t *testing.T) {
	svc, store := newService(t)
	store.List(gomock.Any(), appID, "missing").Return(audience.List{}, audience.ErrNotFound)

	err := svc.AddMembers(t.Context(), appID, "missing", []string{"ana"})
	if !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("AddMembers(unknown list) = %v, want ErrNotFound", err)
	}
}

func TestAddMembersNamesUnknownUsers(t *testing.T) {
	svc, store := newService(t)
	userIDs := []string{"bob", "ana", "cleo", "bob"}
	store.List(gomock.Any(), appID, "list-1").Return(audience.List{ID: "list-1"}, nil)
	store.KnownUsers(gomock.Any(), appID, userIDs).Return([]string{"ana"}, nil)
	// No AddMembers call is expected: one unknown user rejects the whole batch.

	err := svc.AddMembers(t.Context(), appID, "list-1", userIDs)
	if !errors.Is(err, audience.ErrNotFound) {
		t.Fatalf("AddMembers(%q) = %v, want ErrNotFound", userIDs, err)
	}
	if want := `["bob" "cleo"]`; !strings.Contains(err.Error(), want) {
		t.Errorf("AddMembers(%q) = %q, want an error naming the unknown users %s", userIDs, err, want)
	}
}
