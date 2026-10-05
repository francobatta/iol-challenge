package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience/audiencetest"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/token"
)

const testAdminKey = "admin-key"

// A client calls the API of a test server as one app.
type client struct {
	baseURL string
	token   string
}

// call sends a request and returns the response status. A non-nil in is sent as the
// JSON body and a non-nil out receives the decoded JSON response.
func (c client) call(t *testing.T, method, path string, in, out any) int {
	t.Helper()
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Setup: encoding the body of %s %s: %v", method, path, err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, c.baseURL+path, body)
	if err != nil {
		t.Fatalf("Setup: building %s %s: %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
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

// newTestServer starts the API over an empty in-memory store and returns its URL.
func newTestServer(t *testing.T) string {
	t.Helper()
	tokens, err := token.NewSigner("test-secret")
	if err != nil {
		t.Fatalf("Setup: NewSigner failed: %v", err)
	}
	svc := audience.NewService(audiencetest.NewFake())
	srv := httptest.NewServer(NewHandler(svc, tokens, testAdminKey))
	t.Cleanup(srv.Close)
	return srv.URL
}

// createApp registers an app with the admin key and returns the response.
func createApp(t *testing.T, baseURL, adminKey, name string) (status int, token string) {
	t.Helper()
	body := bytes.NewBufferString(fmt.Sprintf(`{"name": %q}`, name))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/v1/apps", body)
	if err != nil {
		t.Fatalf("Setup: building POST /v1/apps: %v", err)
	}
	req.Header.Set("X-Admin-Key", adminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/apps failed: %v", err)
	}
	defer resp.Body.Close()
	var app struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		t.Fatalf("POST /v1/apps: decoding the %d response: %v", resp.StatusCode, err)
	}
	return resp.StatusCode, app.Token
}

// newTestApp starts a server and returns a client for a freshly created app.
func newTestApp(t *testing.T) client {
	t.Helper()
	baseURL := newTestServer(t)
	status, tok := createApp(t, baseURL, testAdminKey, "test app")
	if status != http.StatusCreated {
		t.Fatalf("Setup: POST /v1/apps = %d, want %d", status, http.StatusCreated)
	}
	return client{baseURL: baseURL, token: tok}
}

// A step is one request of a scripted exchange and the status it must produce.
type step struct {
	method, path string
	body         any
	want         int
}

func runSteps(t *testing.T, c client, steps []step) {
	t.Helper()
	for _, s := range steps {
		if got := c.call(t, s.method, s.path, s.body, nil); got != s.want {
			t.Errorf("%s %s with body %v = %d, want %d", s.method, s.path, s.body, got, s.want)
		}
	}
}

type object = map[string]any

func TestCreateApp(t *testing.T) {
	baseURL := newTestServer(t)

	if status, _ := createApp(t, baseURL, "wrong-key", "app"); status != http.StatusUnauthorized {
		t.Errorf("POST /v1/apps with a wrong admin key = %d, want %d", status, http.StatusUnauthorized)
	}
	if status, _ := createApp(t, baseURL, testAdminKey, ""); status != http.StatusBadRequest {
		t.Errorf("POST /v1/apps with an empty name = %d, want %d", status, http.StatusBadRequest)
	}

	status, tok := createApp(t, baseURL, testAdminKey, "app")
	if status != http.StatusCreated {
		t.Fatalf("POST /v1/apps = %d, want %d", status, http.StatusCreated)
	}
	c := client{baseURL: baseURL, token: tok}
	if got := c.call(t, "GET", "/v1/users", nil, nil); got != http.StatusOK {
		t.Errorf("GET /v1/users with the token of the new app = %d, want %d", got, http.StatusOK)
	}
}

func TestRequestsWithoutValidTokenAreRejected(t *testing.T) {
	c := newTestApp(t)
	for _, tok := range []string{"", "garbage", c.token + "x"} {
		bad := client{baseURL: c.baseURL, token: tok}
		var got errorBody
		status := bad.call(t, "GET", "/v1/users", nil, &got)
		if status != http.StatusUnauthorized || got.Error.Code != "unauthorized" {
			t.Errorf("GET /v1/users with token %q = %d, code %q, want %d, code %q",
				tok, status, got.Error.Code, http.StatusUnauthorized, "unauthorized")
		}
	}
}

func TestAppsCannotSeeEachOthersData(t *testing.T) {
	a := newTestApp(t)
	_, tok := createApp(t, a.baseURL, testAdminKey, "other app")
	b := client{baseURL: a.baseURL, token: tok}

	runSteps(t, a, []step{{method: "PUT", path: "/v1/users/ana", want: http.StatusCreated}})
	runSteps(t, b, []step{
		{method: "GET", path: "/v1/users/ana", want: http.StatusNotFound},
		{method: "DELETE", path: "/v1/users/ana", want: http.StatusNotFound},
		// The same ID is a different user in another app.
		{method: "PUT", path: "/v1/users/ana", want: http.StatusCreated},
	})
}

func TestUsers(t *testing.T) {
	c := newTestApp(t)
	runSteps(t, c, []step{
		{method: "GET", path: "/v1/users/ana", want: http.StatusNotFound},
		{method: "PUT", path: "/v1/users/ana", want: http.StatusCreated},
		{method: "PUT", path: "/v1/users/ana", want: http.StatusOK},
		{method: "GET", path: "/v1/users/ana", want: http.StatusOK},
		{method: "DELETE", path: "/v1/users/ana", want: http.StatusNoContent},
		{method: "GET", path: "/v1/users/ana", want: http.StatusNotFound},
		{method: "DELETE", path: "/v1/users/ana", want: http.StatusNotFound},
	})
}

func TestPaging(t *testing.T) {
	c := newTestApp(t)
	for _, id := range []string{"c", "a", "b"} {
		if got := c.call(t, "PUT", "/v1/users/"+id, nil, nil); got != http.StatusCreated {
			t.Fatalf("Setup: PUT /v1/users/%s = %d, want %d", id, got, http.StatusCreated)
		}
	}

	type page struct {
		Items     []audience.User `json:"items"`
		NextAfter string          `json:"next_after"`
	}
	ignoreTimes := cmpopts.IgnoreFields(audience.User{}, "CreatedAt")
	tests := []struct {
		path string
		want page
	}{
		{path: "/v1/users?limit=2", want: page{Items: []audience.User{{ID: "a"}, {ID: "b"}}, NextAfter: "b"}},
		{path: "/v1/users?limit=2&after=b", want: page{Items: []audience.User{{ID: "c"}}}},
		{path: "/v1/users?limit=3", want: page{Items: []audience.User{{ID: "a"}, {ID: "b"}, {ID: "c"}}}},
		{path: "/v1/users?after=c", want: page{Items: []audience.User{}}},
	}
	for _, test := range tests {
		var got page
		if status := c.call(t, "GET", test.path, nil, &got); status != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", test.path, status, http.StatusOK)
			continue
		}
		if diff := cmp.Diff(test.want, got, ignoreTimes); diff != "" {
			t.Errorf("GET %s returned unexpected diff (-want +got):\n%s", test.path, diff)
		}
	}

	for _, limit := range []string{"0", "201", "x"} {
		path := "/v1/users?limit=" + limit
		if got := c.call(t, "GET", path, nil, nil); got != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want %d", path, got, http.StatusBadRequest)
		}
	}
}

func TestEndpoints(t *testing.T) {
	c := newTestApp(t)
	email := object{"address": "ana@example.com", "channel": "email", "provider": "ses"}
	runSteps(t, c, []step{
		{method: "POST", path: "/v1/users/ana/endpoints", body: email, want: http.StatusNotFound},
		{method: "GET", path: "/v1/users/ana/endpoints", want: http.StatusNotFound},
		{method: "PUT", path: "/v1/users/ana", want: http.StatusCreated},
		{method: "POST", path: "/v1/users/ana/endpoints", body: object{"address": "x", "channel": "fax", "provider": "p"}, want: http.StatusBadRequest},
		{method: "POST", path: "/v1/users/ana/endpoints", body: object{"address": "", "channel": "sms", "provider": "p"}, want: http.StatusBadRequest},
		{method: "POST", path: "/v1/users/ana/endpoints", body: object{"address": "x", "channel": "sms"}, want: http.StatusBadRequest},
		{method: "POST", path: "/v1/users/ana/endpoints", body: object{"unknown": 1}, want: http.StatusBadRequest},
	})

	var created audience.Endpoint
	if got := c.call(t, "POST", "/v1/users/ana/endpoints", email, &created); got != http.StatusCreated {
		t.Fatalf("POST /v1/users/ana/endpoints = %d, want %d", got, http.StatusCreated)
	}
	want := audience.Endpoint{ID: created.ID, UserID: "ana", Address: "ana@example.com", Channel: audience.ChannelEmail, Provider: "ses"}
	if diff := cmp.Diff(want, created); diff != "" || created.ID == "" {
		t.Errorf("POST /v1/users/ana/endpoints returned ID %q and unexpected diff (-want +got):\n%s", created.ID, diff)
	}

	path := "/v1/endpoints/" + created.ID
	var updated audience.Endpoint
	if got := c.call(t, "PATCH", path, object{"provider": "sendgrid"}, &updated); got != http.StatusOK {
		t.Fatalf("PATCH %s = %d, want %d", path, got, http.StatusOK)
	}
	want.Provider = "sendgrid"
	if diff := cmp.Diff(want, updated); diff != "" {
		t.Errorf("PATCH %s returned unexpected diff (-want +got):\n%s", path, diff)
	}

	runSteps(t, c, []step{
		{method: "POST", path: "/v1/users/ana/endpoints", body: email, want: http.StatusConflict},
		{method: "PATCH", path: path, body: object{"channel": "fax"}, want: http.StatusBadRequest},
		{method: "PATCH", path: "/v1/endpoints/missing", body: object{"address": "x"}, want: http.StatusNotFound},
		{method: "GET", path: path, want: http.StatusOK},
		{method: "GET", path: "/v1/users/ana/endpoints", want: http.StatusOK},
		// Deleting the user takes its endpoints with it.
		{method: "DELETE", path: "/v1/users/ana", want: http.StatusNoContent},
		{method: "GET", path: path, want: http.StatusNotFound},
		{method: "DELETE", path: path, want: http.StatusNotFound},
	})
}

func TestLists(t *testing.T) {
	c := newTestApp(t)
	var created audience.List
	body := object{"name": "beta testers", "description": "early access"}
	if got := c.call(t, "POST", "/v1/lists", body, &created); got != http.StatusCreated {
		t.Fatalf("POST /v1/lists = %d, want %d", got, http.StatusCreated)
	}
	if created.ID == "" || created.Name != "beta testers" || created.Description != "early access" {
		t.Errorf("POST /v1/lists = %+v, want an ID, name %q and description %q", created, "beta testers", "early access")
	}

	path := "/v1/lists/" + created.ID
	var updated audience.List
	if got := c.call(t, "PATCH", path, object{"name": "beta"}, &updated); got != http.StatusOK {
		t.Fatalf("PATCH %s = %d, want %d", path, got, http.StatusOK)
	}
	if updated.Name != "beta" || updated.Description != "early access" {
		t.Errorf("PATCH %s with a new name = %+v, want name %q and description unchanged", path, updated, "beta")
	}

	runSteps(t, c, []step{
		{method: "POST", path: "/v1/lists", body: object{"name": ""}, want: http.StatusBadRequest},
		{method: "POST", path: "/v1/lists", body: object{"name": "beta"}, want: http.StatusConflict},
		{method: "POST", path: "/v1/lists", body: object{"name": "other"}, want: http.StatusCreated},
		{method: "PATCH", path: path, body: object{"name": "other"}, want: http.StatusConflict},
		{method: "PATCH", path: path, body: object{"name": ""}, want: http.StatusBadRequest},
		{method: "GET", path: path, want: http.StatusOK},
		{method: "GET", path: "/v1/lists", want: http.StatusOK},
		{method: "DELETE", path: path, want: http.StatusNoContent},
		{method: "GET", path: path, want: http.StatusNotFound},
		{method: "DELETE", path: path, want: http.StatusNotFound},
	})
}

func TestMembers(t *testing.T) {
	c := newTestApp(t)
	var list audience.List
	if got := c.call(t, "POST", "/v1/lists", object{"name": "beta"}, &list); got != http.StatusCreated {
		t.Fatalf("Setup: POST /v1/lists = %d, want %d", got, http.StatusCreated)
	}
	members := "/v1/lists/" + list.ID + "/members"

	runSteps(t, c, []step{
		{method: "PUT", path: "/v1/users/ana", want: http.StatusCreated},
		{method: "PUT", path: "/v1/users/bob", want: http.StatusCreated},
		{method: "PUT", path: "/v1/users/cleo", want: http.StatusCreated},

		{method: "PUT", path: members + "/nobody", want: http.StatusNotFound},
		{method: "PUT", path: "/v1/lists/missing/members/ana", want: http.StatusNotFound},
		{method: "GET", path: "/v1/lists/missing/members", want: http.StatusNotFound},
		{method: "PUT", path: members + "/ana", want: http.StatusNoContent},
		{method: "PUT", path: members + "/ana", want: http.StatusNoContent},

		{method: "POST", path: members, body: object{"user_ids": []string{}}, want: http.StatusBadRequest},
		// One unknown user rejects the whole batch, so bob is not added here.
		{method: "POST", path: members, body: object{"user_ids": []string{"bob", "nobody"}}, want: http.StatusNotFound},
	})

	type page struct {
		Items []audience.Member `json:"items"`
	}
	ignoreTimes := cmpopts.IgnoreFields(audience.Member{}, "AddedAt")
	wantMembers := func(when string, want ...audience.Member) {
		t.Helper()
		var got page
		if status := c.call(t, "GET", members, nil, &got); status != http.StatusOK {
			t.Fatalf("GET %s %s = %d, want %d", members, when, status, http.StatusOK)
		}
		if diff := cmp.Diff(page{Items: want}, got, ignoreTimes, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("GET %s %s returned unexpected diff (-want +got):\n%s", members, when, diff)
		}
	}
	wantMembers("after a rejected batch", audience.Member{UserID: "ana"})

	var notFound errorBody
	c.call(t, "POST", members, object{"user_ids": []string{"nobody"}}, &notFound)
	if want := `not found: users ["nobody"]`; notFound.Error.Message != want {
		t.Errorf("POST %s with an unknown user returned message %q, want %q", members, notFound.Error.Message, want)
	}

	runSteps(t, c, []step{
		{method: "POST", path: members, body: object{"user_ids": []string{"ana", "bob", "cleo"}}, want: http.StatusNoContent},
	})
	wantMembers("after a batch", audience.Member{UserID: "ana"}, audience.Member{UserID: "bob"}, audience.Member{UserID: "cleo"})

	runSteps(t, c, []step{
		{method: "DELETE", path: members + "/bob", want: http.StatusNoContent},
		{method: "DELETE", path: members + "/bob", want: http.StatusNoContent},
		// Deleting a user takes it out of its lists.
		{method: "DELETE", path: "/v1/users/cleo", want: http.StatusNoContent},
	})
	wantMembers("after removals", audience.Member{UserID: "ana"})
}
