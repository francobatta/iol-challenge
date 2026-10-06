package main

import "net/http"

type dependencies struct {
	router http.Handler
}

// newDependencies builds the mock. It opens nothing, so there is nothing to close.
func newDependencies(cfg config) *dependencies {
	return &dependencies{router: newRouter(cfg.ErrorRate, cfg.ThrottleRate)}
}
