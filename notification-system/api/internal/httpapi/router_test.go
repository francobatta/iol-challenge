package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience/audiencetest"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight/insighttest"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify/notifytest"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/token"
)

const (
	testAdminKey = "admin-key"
	testAppID    = "app-1"
)

// A client calls a test server with the credentials it holds.
type client struct {
	baseURL        string
	token          string
	adminKey       string
	idempotencyKey string
}

// call sends a request and returns the response status. A non-empty body is sent as
// it is, and a non-nil out receives the decoded JSON response.
func (c client) call(t *testing.T, method, path, body string, out any) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, c.baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("Setup: building %s %s: %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Admin-Key", c.adminKey)
	if c.idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", c.idempotencyKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decoding the %d response: %v", method, path, resp.StatusCode, err)
		}
	}
	return resp.StatusCode
}

// newTestAPI starts the API over a mock repository. It returns a client holding a token
// for testAppID, and the recorder on which a test states the repository calls it expects.
// Any other repository call fails the test.
func newTestAPI(t *testing.T) (client, *audiencetest.MockRepositoryMockRecorder) {
	t.Helper()
	c, m := startTestAPI(t)
	return c, m.audience
}

// The mocks behind a test server. A test states the calls it expects on them; any
// other call fails the test.
type mocks struct {
	audience *audiencetest.MockRepositoryMockRecorder
	jobs     *notifytest.MockRepositoryMockRecorder
	prom     *insighttest.MockQuerierMockRecorder
}

// startTestAPI starts the API over mocks and returns a client holding a token for
// testAppID.
func startTestAPI(t *testing.T) (client, mocks) {
	t.Helper()
	tokens, err := token.NewSigner("test-secret")
	if err != nil {
		t.Fatalf("Setup: NewSigner failed: %v", err)
	}
	tok, err := tokens.Issue(testAppID)
	if err != nil {
		t.Fatalf("Setup: Issue(%q) failed: %v", testAppID, err)
	}
	ctrl := gomock.NewController(t)
	repo, jobs, prom := audiencetest.NewMockRepository(ctrl), notifytest.NewMockRepository(ctrl), insighttest.NewMockQuerier(ctrl)
	srv := httptest.NewServer(NewRouter(audience.NewService(repo), notify.NewService(jobs), insight.NewService(prom), tokens, testAdminKey))
	t.Cleanup(srv.Close)
	return client{baseURL: srv.URL, token: tok}, mocks{audience: repo.EXPECT(), jobs: jobs.EXPECT(), prom: prom.EXPECT()}
}

func TestStatus(t *testing.T) {
	ctx := gomock.Any()
	var (
		ana   = audience.User{ID: "ana"}
		email = audience.Endpoint{ID: "e1", UserID: "ana", Address: "ana@example.com", Channel: audience.ChannelEmail, Provider: "mailchimp"}
		beta  = audience.List{ID: "l1", Name: "beta", Description: "early access"}
	)
	const emailBody = `{"address": "ana@example.com", "channel": "email", "provider": "mailchimp"}`

	tests := []struct {
		name               string
		method, path, body string
		expect             func(repo *audiencetest.MockRepositoryMockRecorder)
		want               int
	}{
		{
			name: "RegisterNewUser", method: "PUT", path: "/v1/users/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.PutUser(ctx, testAppID, "ana").Return(ana, true, nil)
			},
			want: http.StatusCreated,
		},
		{
			name: "RegisterExistingUser", method: "PUT", path: "/v1/users/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.PutUser(ctx, testAppID, "ana").Return(ana, false, nil)
			},
			want: http.StatusOK,
		},
		{
			name: "RegisterUserWithTooLongID", method: "PUT", path: "/v1/users/" + strings.Repeat("a", audience.MaxUserIDLen+1),
			want: http.StatusBadRequest,
		},
		{
			name: "GetUser", method: "GET", path: "/v1/users/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.User(ctx, testAppID, "ana").Return(ana, nil)
			},
			want: http.StatusOK,
		},
		{
			name: "GetUnknownUser", method: "GET", path: "/v1/users/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.User(ctx, testAppID, "ana").Return(audience.User{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound,
		},
		{
			name: "StoreFailure", method: "GET", path: "/v1/users/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.User(ctx, testAppID, "ana").Return(audience.User{}, errors.New("database is down"))
			},
			want: http.StatusInternalServerError,
		},
		{
			name: "DeleteUser", method: "DELETE", path: "/v1/users/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.DeleteUser(ctx, testAppID, "ana").Return(nil)
			},
			want: http.StatusNoContent,
		},
		{
			name: "DeleteUnknownUser", method: "DELETE", path: "/v1/users/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.DeleteUser(ctx, testAppID, "ana").Return(audience.ErrNotFound)
			},
			want: http.StatusNotFound,
		},

		{
			name: "CreateEndpoint", method: "POST", path: "/v1/users/ana/endpoints", body: emailBody,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.CreateEndpoint(ctx, testAppID, audience.Endpoint{
					UserID: "ana", Address: "ana@example.com", Channel: audience.ChannelEmail, Provider: "mailchimp",
				}).Return(email, nil)
			},
			want: http.StatusCreated,
		},
		{
			name: "CreateEndpointForUnknownUser", method: "POST", path: "/v1/users/ana/endpoints", body: emailBody,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.CreateEndpoint(ctx, testAppID, gomock.Any()).Return(audience.Endpoint{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound,
		},
		{
			name: "CreateDuplicateEndpoint", method: "POST", path: "/v1/users/ana/endpoints", body: emailBody,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.CreateEndpoint(ctx, testAppID, gomock.Any()).Return(audience.Endpoint{}, audience.ErrConflict)
			},
			want: http.StatusConflict,
		},
		{
			name: "CreateEndpointWithUnknownChannel", method: "POST", path: "/v1/users/ana/endpoints",
			body: `{"address": "x", "channel": "fax", "provider": "twilio"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "CreateEndpointWithoutAddress", method: "POST", path: "/v1/users/ana/endpoints",
			body: `{"channel": "sms", "provider": "twilio"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "CreateEndpointWithoutProvider", method: "POST", path: "/v1/users/ana/endpoints",
			body: `{"address": "x", "channel": "sms"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "CreateEndpointWithUnknownField", method: "POST", path: "/v1/users/ana/endpoints",
			body: `{"address": "x", "channel": "sms", "provider": "twilio", "extra": 1}`,
			want: http.StatusBadRequest,
		},
		{
			name: "CreateEndpointWithMalformedBody", method: "POST", path: "/v1/users/ana/endpoints",
			body: `{"address": `,
			want: http.StatusBadRequest,
		},
		{
			name: "GetEndpoint", method: "GET", path: "/v1/endpoints/e1",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.Endpoint(ctx, testAppID, "e1").Return(email, nil)
			},
			want: http.StatusOK,
		},
		{
			name: "ListEndpointsOfUnknownUser", method: "GET", path: "/v1/users/ana/endpoints",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.User(ctx, testAppID, "ana").Return(audience.User{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound,
		},
		{
			name: "UpdateEndpoint", method: "PATCH", path: "/v1/endpoints/e1", body: `{"address": "new@example.com"}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				// Only the address changes; the other fields keep their stored values.
				updated := email
				updated.Address = "new@example.com"
				repo.Endpoint(ctx, testAppID, "e1").Return(email, nil)
				repo.UpdateEndpoint(ctx, testAppID, updated).Return(updated, nil)
			},
			want: http.StatusOK,
		},
		{
			name: "UpdateEndpointToProviderOfAnotherChannel", method: "PATCH", path: "/v1/endpoints/e1", body: `{"provider": "twilio"}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.Endpoint(ctx, testAppID, "e1").Return(email, nil)
			},
			want: http.StatusBadRequest,
		},
		{
			name: "CreateEndpointWithProviderOfAnotherChannel", method: "POST", path: "/v1/users/ana/endpoints",
			body: `{"address": "x", "channel": "sms", "provider": "fcm"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "UpdateEndpointToUnknownChannel", method: "PATCH", path: "/v1/endpoints/e1", body: `{"channel": "fax"}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.Endpoint(ctx, testAppID, "e1").Return(email, nil)
			},
			want: http.StatusBadRequest,
		},
		{
			name: "UpdateUnknownEndpoint", method: "PATCH", path: "/v1/endpoints/e1", body: `{"address": "x"}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.Endpoint(ctx, testAppID, "e1").Return(audience.Endpoint{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound,
		},
		{
			name: "DeleteEndpoint", method: "DELETE", path: "/v1/endpoints/e1",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.DeleteEndpoint(ctx, testAppID, "e1").Return(nil)
			},
			want: http.StatusNoContent,
		},

		{
			name: "CreateList", method: "POST", path: "/v1/lists", body: `{"name": "beta", "description": "early access"}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.CreateList(ctx, testAppID, audience.List{Name: "beta", Description: "early access"}).Return(beta, nil)
			},
			want: http.StatusCreated,
		},
		{
			name: "CreateListWithoutName", method: "POST", path: "/v1/lists", body: `{"description": "early access"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "CreateListWithTakenName", method: "POST", path: "/v1/lists", body: `{"name": "beta"}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.CreateList(ctx, testAppID, gomock.Any()).Return(audience.List{}, audience.ErrConflict)
			},
			want: http.StatusConflict,
		},
		{
			name: "GetList", method: "GET", path: "/v1/lists/l1",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.List(ctx, testAppID, "l1").Return(beta, nil)
			},
			want: http.StatusOK,
		},
		{
			name: "UpdateList", method: "PATCH", path: "/v1/lists/l1", body: `{"name": "testers"}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				// Only the name changes; the description keeps its stored value.
				updated := beta
				updated.Name = "testers"
				repo.List(ctx, testAppID, "l1").Return(beta, nil)
				repo.UpdateList(ctx, testAppID, updated).Return(updated, nil)
			},
			want: http.StatusOK,
		},
		{
			name: "UpdateListToEmptyName", method: "PATCH", path: "/v1/lists/l1", body: `{"name": ""}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.List(ctx, testAppID, "l1").Return(beta, nil)
			},
			want: http.StatusBadRequest,
		},
		{
			name: "DeleteList", method: "DELETE", path: "/v1/lists/l1",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.DeleteList(ctx, testAppID, "l1").Return(nil)
			},
			want: http.StatusNoContent,
		},

		{
			name: "AddMember", method: "PUT", path: "/v1/lists/l1/members/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.List(ctx, testAppID, "l1").Return(beta, nil)
				repo.KnownUsers(ctx, testAppID, []string{"ana"}).Return([]string{"ana"}, nil)
				repo.AddMembers(ctx, testAppID, "l1", []string{"ana"}).Return(nil)
			},
			want: http.StatusNoContent,
		},
		{
			name: "AddUnknownUserAsMember", method: "PUT", path: "/v1/lists/l1/members/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.List(ctx, testAppID, "l1").Return(beta, nil)
				repo.KnownUsers(ctx, testAppID, []string{"ana"}).Return(nil, nil)
			},
			want: http.StatusNotFound,
		},
		{
			name: "AddMemberToUnknownList", method: "PUT", path: "/v1/lists/l1/members/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.List(ctx, testAppID, "l1").Return(audience.List{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound,
		},
		{
			name: "AddMembers", method: "POST", path: "/v1/lists/l1/members", body: `{"user_ids": ["ana", "bob"]}`,
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.List(ctx, testAppID, "l1").Return(beta, nil)
				repo.KnownUsers(ctx, testAppID, []string{"ana", "bob"}).Return([]string{"ana", "bob"}, nil)
				repo.AddMembers(ctx, testAppID, "l1", []string{"ana", "bob"}).Return(nil)
			},
			want: http.StatusNoContent,
		},
		{
			name: "AddNoMembers", method: "POST", path: "/v1/lists/l1/members", body: `{"user_ids": []}`,
			want: http.StatusBadRequest,
		},
		{
			name: "RemoveMember", method: "DELETE", path: "/v1/lists/l1/members/ana",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.RemoveMember(ctx, testAppID, "l1", "ana").Return(nil)
			},
			want: http.StatusNoContent,
		},
		{
			name: "ListMembersOfUnknownList", method: "GET", path: "/v1/lists/l1/members",
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.List(ctx, testAppID, "l1").Return(audience.List{}, audience.ErrNotFound)
			},
			want: http.StatusNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, repo := newTestAPI(t)
			if test.expect != nil {
				test.expect(repo)
			}
			if got := c.call(t, test.method, test.path, test.body, nil); got != test.want {
				t.Errorf("%s %s with body %q = %d, want %d", test.method, test.path, test.body, got, test.want)
			}
		})
	}
}

func TestCreateApp(t *testing.T) {
	c, repo := newTestAPI(t)
	c.adminKey = testAdminKey
	c.token = ""
	repo.CreateApp(gomock.Any(), "my app").Return(audience.App{ID: "app-2", Name: "my app"}, nil)

	var created struct {
		AppID string `json:"app_id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if got := c.call(t, "POST", "/v1/apps", `{"name": "my app"}`, &created); got != http.StatusCreated {
		t.Fatalf("POST /v1/apps = %d, want %d", got, http.StatusCreated)
	}
	if created.AppID != "app-2" || created.Name != "my app" {
		t.Errorf("POST /v1/apps = %+v, want app_id %q and name %q", created, "app-2", "my app")
	}

	// The returned token must act as the new app: the repository is asked for app-2's users.
	repo.Users(gomock.Any(), "app-2", gomock.Any()).Return(nil, nil)
	asApp := client{baseURL: c.baseURL, token: created.Token}
	if got := asApp.call(t, "GET", "/v1/users", "", nil); got != http.StatusOK {
		t.Errorf("GET /v1/users with the token of the new app = %d, want %d", got, http.StatusOK)
	}
}

func TestCreateAppRejects(t *testing.T) {
	tests := []struct {
		name     string
		adminKey string
		body     string
		want     int
	}{
		{name: "a missing admin key", adminKey: "", body: `{"name": "app"}`, want: http.StatusUnauthorized},
		{name: "a wrong admin key", adminKey: "wrong", body: `{"name": "app"}`, want: http.StatusUnauthorized},
		{name: "an empty name", adminKey: testAdminKey, body: `{"name": ""}`, want: http.StatusBadRequest},
	}
	for _, test := range tests {
		c, _ := newTestAPI(t)
		c.adminKey = test.adminKey
		if got := c.call(t, "POST", "/v1/apps", test.body, nil); got != test.want {
			t.Errorf("POST /v1/apps with %s = %d, want %d", test.name, got, test.want)
		}
	}
}

func TestRequestsWithoutValidTokenAreRejected(t *testing.T) {
	c, _ := newTestAPI(t)
	for _, tok := range []string{"", "garbage", c.token + "x"} {
		bad := client{baseURL: c.baseURL, token: tok}
		var got errorBody
		status := bad.call(t, "GET", "/v1/users", "", &got)
		if status != http.StatusUnauthorized || got.Error.Code != "unauthorized" {
			t.Errorf("GET /v1/users with token %q = %d, code %q, want %d, code %q",
				tok, status, got.Error.Code, http.StatusUnauthorized, "unauthorized")
		}
	}
}

func TestUnknownRoutes(t *testing.T) {
	c, _ := newTestAPI(t)
	if got := c.call(t, "GET", "/v1/nothing", "", nil); got != http.StatusNotFound {
		t.Errorf("GET /v1/nothing with a token = %d, want %d", got, http.StatusNotFound)
	}
	// Without a token an unknown route looks like any other, so the API does not
	// reveal which routes exist.
	c.token = ""
	if got := c.call(t, "GET", "/v1/nothing", "", nil); got != http.StatusUnauthorized {
		t.Errorf("GET /v1/nothing without a token = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestAppHandlerRefusesToRunWithoutAuthentication(t *testing.T) {
	called := false
	h := appHandler(func(w http.ResponseWriter, r *http.Request, appID string) error {
		called = true
		return nil
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/users", nil))

	if called || rec.Code != http.StatusInternalServerError {
		t.Errorf("appHandler outside authenticateApp: handler called = %t, status = %d, want false, %d",
			called, rec.Code, http.StatusInternalServerError)
	}
}

func TestPaging(t *testing.T) {
	type page struct {
		Items     []audience.User `json:"items"`
		NextAfter string          `json:"next_after"`
	}
	a, b, c := audience.User{ID: "a"}, audience.User{ID: "b"}, audience.User{ID: "c"}

	// The repository is always asked for one user more than the page holds, and that extra
	// user, when it comes back, is what produces next_after.
	tests := []struct {
		name      string
		path      string
		wantAsked audience.Page
		stored    []audience.User
		want      page
	}{
		{
			name: "DefaultLimit", path: "/v1/users",
			wantAsked: audience.Page{Limit: defaultPageSize + 1},
			stored:    []audience.User{a, b},
			want:      page{Items: []audience.User{a, b}},
		},
		{
			name: "MorePagesFollow", path: "/v1/users?limit=2",
			wantAsked: audience.Page{Limit: 3},
			stored:    []audience.User{a, b, c},
			want:      page{Items: []audience.User{a, b}, NextAfter: "b"},
		},
		{
			name: "LastPage", path: "/v1/users?limit=2&after=b",
			wantAsked: audience.Page{After: "b", Limit: 3},
			stored:    []audience.User{c},
			want:      page{Items: []audience.User{c}},
		},
		{
			name: "Empty", path: "/v1/users?after=c",
			wantAsked: audience.Page{After: "c", Limit: defaultPageSize + 1},
			stored:    nil,
			want:      page{Items: []audience.User{}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cl, repo := newTestAPI(t)
			repo.Users(gomock.Any(), testAppID, test.wantAsked).Return(test.stored, nil)

			var got page
			if status := cl.call(t, "GET", test.path, "", &got); status != http.StatusOK {
				t.Fatalf("GET %s = %d, want %d", test.path, status, http.StatusOK)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("GET %s returned unexpected diff (-want +got):\n%s", test.path, diff)
			}
		})
	}
}

func TestPagingRejectsBadLimits(t *testing.T) {
	c, _ := newTestAPI(t)
	for _, limit := range []string{"0", "201", "-1", "x"} {
		path := "/v1/users?limit=" + limit
		if got := c.call(t, "GET", path, "", nil); got != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want %d", path, got, http.StatusBadRequest)
		}
	}
}

func TestEndpointBody(t *testing.T) {
	c, repo := newTestAPI(t)
	want := audience.Endpoint{ID: "e1", UserID: "ana", Address: "+5491100000000", Channel: audience.ChannelSMS, Provider: "twilio"}
	repo.Endpoint(gomock.Any(), testAppID, "e1").Return(want, nil)

	var got map[string]any
	if status := c.call(t, "GET", "/v1/endpoints/e1", "", &got); status != http.StatusOK {
		t.Fatalf("GET /v1/endpoints/e1 = %d, want %d", status, http.StatusOK)
	}
	wantBody := map[string]any{
		"endpoint_id": "e1",
		"user_id":     "ana",
		"address":     "+5491100000000",
		"channel":     "sms",
		"provider":    "twilio",
	}
	if diff := cmp.Diff(wantBody, got); diff != "" {
		t.Errorf("GET /v1/endpoints/e1 returned unexpected diff (-want +got):\n%s", diff)
	}
}

func TestErrorBody(t *testing.T) {
	c, repo := newTestAPI(t)
	repo.List(gomock.Any(), testAppID, "l1").Return(audience.List{ID: "l1"}, nil)
	repo.KnownUsers(gomock.Any(), testAppID, []string{"ana", "nobody"}).Return([]string{"ana"}, nil)

	var got errorBody
	c.call(t, "POST", "/v1/lists/l1/members", `{"user_ids": ["ana", "nobody"]}`, &got)
	want := errorBody{Error: errorDetail{Code: "not_found", Message: `not found: users ["nobody"]`}}
	if got != want {
		t.Errorf("POST /v1/lists/l1/members with an unknown user returned %+v, want %+v", got, want)
	}
}

func TestInternalErrorsHideTheirCause(t *testing.T) {
	c, repo := newTestAPI(t)
	repo.User(gomock.Any(), testAppID, "ana").Return(audience.User{}, errors.New("password=hunter2 rejected"))

	var got errorBody
	c.call(t, "GET", "/v1/users/ana", "", &got)
	want := errorBody{Error: errorDetail{Code: "internal", Message: "internal error"}}
	if got != want {
		t.Errorf("GET /v1/users/ana with a failing repository returned %+v, want %+v", got, want)
	}
}
