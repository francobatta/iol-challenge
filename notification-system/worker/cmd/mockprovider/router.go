package main

import (
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/francobatta/iol-challenge/notification-system/commons/health"
)

const (
	minLatency = time.Millisecond
	maxLatency = time.Second
)

// newRouter returns the mock of all four providers. The routes are the ones the
// worker's provider clients post to.
func newRouter(errorRate, throttleRate float64) http.Handler {
	answer := func(w http.ResponseWriter, r *http.Request) {
		// Timing and failures do not need to be unpredictable, only varied.
		latency := minLatency + rand.N(maxLatency-minLatency)
		select {
		case <-time.After(latency):
		case <-r.Context().Done():
			return // the client gave up
		}
		switch roll := rand.Float64(); {
		case roll < throttleRate:
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case roll < throttleRate+errorRate:
			http.Error(w, "something broke", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}

	r := chi.NewRouter()
	health.Mount(r, nil) // nothing to wait for: it is ready as soon as it listens
	r.Post("/twilio", answer)
	r.Post("/mailchimp", answer)
	r.Post("/apns/{device_token}", answer)
	r.Post("/fcm", answer)
	return r
}
