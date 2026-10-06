package httpapi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight"
)

func TestMetrics(t *testing.T) {
	c, m := startTestAPI(t)
	m.prom.Instant(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)
	// An hour in sixty points: the range asked for is the one queried.
	m.prom.Range(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).MinTimes(1).Return(nil, nil)

	var got insight.Snapshot
	if status := c.call(t, "GET", "/v1/metrics?range=1h", "", &got); status != http.StatusOK {
		t.Fatalf("GET /v1/metrics?range=1h = %d, want %d", status, http.StatusOK)
	}
	if got.WindowSeconds != 3600 || len(got.App.Providers) != 4 {
		t.Errorf("GET /v1/metrics?range=1h returned a window of %ds and %d providers, want 3600s and 4", got.WindowSeconds, len(got.App.Providers))
	}
}

func TestMetricsStatus(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		noToken bool
		promErr error
		want    int
	}{
		{name: "DefaultRange", path: "/v1/metrics", want: http.StatusOK},
		{name: "NoToken", path: "/v1/metrics", noToken: true, want: http.StatusUnauthorized},
		{name: "RangeNotADuration", path: "/v1/metrics?range=soon", want: http.StatusBadRequest},
		{name: "RangeTooLong", path: "/v1/metrics?range=720h", want: http.StatusBadRequest},
		{name: "PrometheusDown", path: "/v1/metrics", promErr: errors.New("connection refused"), want: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, m := startTestAPI(t)
			if test.noToken {
				c.token = ""
			}
			m.prom.Instant(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, test.promErr)
			m.prom.Range(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, test.promErr)
			if got := c.call(t, "GET", test.path, "", nil); got != test.want {
				t.Errorf("GET %s = %d, want %d", test.path, got, test.want)
			}
		})
	}
}
