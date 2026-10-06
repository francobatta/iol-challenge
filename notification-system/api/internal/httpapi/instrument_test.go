package httpapi

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience/audiencetest"
)

// requestSeries returns what reg holds of the request metrics: the value of every series
// of notify_http_requests_total keyed by "method route status", and the number of
// requests timed by notify_http_request_duration_seconds keyed by "method route".
func requestSeries(t *testing.T, reg *prometheus.Registry) (counts map[string]float64, timed map[string]uint64) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Setup: gathering the metrics: %v", err)
	}
	counts, timed = map[string]float64{}, map[string]uint64{}
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			switch family.GetName() {
			case "notify_http_requests_total":
				counts[labels["method"]+" "+labels["route"]+" "+labels["status"]] = metric.GetCounter().GetValue()
			case "notify_http_request_duration_seconds":
				timed[labels["method"]+" "+labels["route"]] = metric.GetHistogram().GetSampleCount()
			}
		}
	}
	return counts, timed
}

func TestRequestMetrics(t *testing.T) {
	ctx := gomock.Any()
	type request struct{ method, path string }

	tests := []struct {
		name      string
		requests  []request
		noToken   bool
		expect    func(repo *audiencetest.MockRepositoryMockRecorder)
		want      map[string]float64
		wantTimed map[string]uint64
	}{
		{
			name:     "MatchedRoute",
			requests: []request{{"GET", "/v1/users"}},
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.Users(ctx, testAppID, gomock.Any()).Return(nil, nil)
			},
			want:      map[string]float64{"GET /v1/users 200": 1},
			wantTimed: map[string]uint64{"GET /v1/users": 1},
		},
		{
			// The route is the pattern, so the series do not multiply with the users.
			name:     "PathParameter",
			requests: []request{{"GET", "/v1/users/ana"}, {"GET", "/v1/users/bob"}},
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.User(ctx, testAppID, "ana").Return(audience.User{ID: "ana"}, nil)
				repo.User(ctx, testAppID, "bob").Return(audience.User{ID: "bob"}, nil)
			},
			want:      map[string]float64{"GET /v1/users/{user_id} 200": 2},
			wantTimed: map[string]uint64{"GET /v1/users/{user_id}": 2},
		},
		{
			name:      "UnmatchedPath",
			requests:  []request{{"GET", "/v1/nothing"}, {"GET", "/elsewhere"}},
			want:      map[string]float64{"GET unmatched 404": 2},
			wantTimed: map[string]uint64{"GET unmatched": 2},
		},
		{
			// The path has a route, though not for this method.
			name:      "MethodNotAllowed",
			requests:  []request{{"PATCH", "/v1/users"}},
			want:      map[string]float64{"PATCH /v1/users 405": 1},
			wantTimed: map[string]uint64{"PATCH /v1/users": 1},
		},
		{
			// Refused before it reaches the route, and still counted under it.
			name:      "ClientError",
			requests:  []request{{"GET", "/v1/users"}},
			noToken:   true,
			want:      map[string]float64{"GET /v1/users 401": 1},
			wantTimed: map[string]uint64{"GET /v1/users": 1},
		},
		{
			name:     "ServerError",
			requests: []request{{"DELETE", "/v1/users/ana"}},
			expect: func(repo *audiencetest.MockRepositoryMockRecorder) {
				repo.DeleteUser(ctx, testAppID, "ana").Return(errors.New("database is down"))
			},
			want:      map[string]float64{"DELETE /v1/users/{user_id} 500": 1},
			wantTimed: map[string]uint64{"DELETE /v1/users/{user_id}": 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, m := startTestAPI(t)
			if test.noToken {
				c.token = ""
			}
			if test.expect != nil {
				test.expect(m.audience)
			}
			for _, req := range test.requests {
				c.call(t, req.method, req.path, "", nil)
			}

			got, gotTimed := requestSeries(t, m.registry)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("notify_http_requests_total after %v: unexpected diff (-want +got):\n%s", test.requests, diff)
			}
			if diff := cmp.Diff(test.wantTimed, gotTimed); diff != "" {
				t.Errorf("notify_http_request_duration_seconds after %v: unexpected diff in requests timed (-want +got):\n%s", test.requests, diff)
			}
		})
	}
}
