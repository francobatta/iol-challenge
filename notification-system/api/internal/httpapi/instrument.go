package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

// routeUnmatched is the route a request is counted under when no route serves it.
const routeUnmatched = "unmatched"

// Metrics is what the API reports about the requests it answers.
type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notify_http_requests_total",
			Help: "Requests answered by the API, by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "notify_http_request_duration_seconds",
			Help:    "Duration of the requests answered by the API, by method and route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// instrument returns router with every request it answers counted and timed.
//
// A request is counted under the pattern of its route, such as /v1/users/{user_id},
// rather than its path, so the number of series does not grow with the data.
func (m *Metrics) instrument(router chi.Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		router.ServeHTTP(ww, r)

		// The route is looked up here rather than read from what served the request,
		// which would not know it for a request that authentication turned away.
		route := routeUnmatched
		if rctx := chi.NewRouteContext(); router.Match(rctx, r.Method, r.URL.Path) {
			route = rctx.RoutePattern()
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK // the handler wrote nothing
		}
		m.requests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
		m.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}
