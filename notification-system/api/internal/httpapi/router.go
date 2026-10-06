// Package httpapi serves the audience and notification REST API over HTTP.
//
// Every route but app creation is called by an app, which identifies itself with a
// bearer token. The app's ID always comes from that token, never from the URL or body.
//
// The package is the transport layer and nothing more: router.go lists the routes,
// the handlers turn a request into one call on a service and its answer into JSON, and
// respond.go holds how answers and errors are written.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/token"
)

type server struct {
	audiences     *audience.Service
	notifications *notify.Service
	insights      *insight.Service
	tokens        *token.Signer
	adminKey      string
}

// NewRouter returns the handler for the whole API. Requests to create an app must
// carry adminKey in the X-Admin-Key header; all other requests must carry a token
// issued by tokens.
func NewRouter(audiences *audience.Service, notifications *notify.Service, insights *insight.Service, tokens *token.Signer, adminKey string) http.Handler {
	s := &server{audiences: audiences, notifications: notifications, insights: insights, tokens: tokens, adminKey: adminKey}

	r := chi.NewRouter()
	r.With(s.requireAdmin).Method(http.MethodPost, "/v1/apps", handler(s.createApp))

	// Routes called by an app. authenticateApp guards everything under /v1, so a route
	// added here cannot be reached without a token, and without one an unknown route
	// looks like any other.
	r.Route("/v1", func(r chi.Router) {
		r.Use(s.authenticateApp)

		r.Route("/users", func(r chi.Router) {
			r.Method(http.MethodGet, "/", appHandler(s.users))
			r.Method(http.MethodPut, "/{user_id}", appHandler(s.registerUser))
			r.Method(http.MethodGet, "/{user_id}", appHandler(s.user))
			r.Method(http.MethodDelete, "/{user_id}", appHandler(s.deleteUser))
			r.Method(http.MethodPost, "/{user_id}/endpoints", appHandler(s.createEndpoint))
			r.Method(http.MethodGet, "/{user_id}/endpoints", appHandler(s.endpoints))
		})

		r.Route("/endpoints/{endpoint_id}", func(r chi.Router) {
			r.Method(http.MethodGet, "/", appHandler(s.endpoint))
			r.Method(http.MethodPatch, "/", appHandler(s.updateEndpoint))
			r.Method(http.MethodDelete, "/", appHandler(s.deleteEndpoint))
		})

		r.Route("/lists", func(r chi.Router) {
			r.Method(http.MethodPost, "/", appHandler(s.createList))
			r.Method(http.MethodGet, "/", appHandler(s.lists))
			r.Method(http.MethodGet, "/{list_id}", appHandler(s.list))
			r.Method(http.MethodPatch, "/{list_id}", appHandler(s.updateList))
			r.Method(http.MethodDelete, "/{list_id}", appHandler(s.deleteList))
			r.Method(http.MethodPost, "/{list_id}/members", appHandler(s.addMembers))
			r.Method(http.MethodGet, "/{list_id}/members", appHandler(s.members))
			r.Method(http.MethodPut, "/{list_id}/members/{user_id}", appHandler(s.addMember))
			r.Method(http.MethodDelete, "/{list_id}/members/{user_id}", appHandler(s.removeMember))
		})

		r.Route("/notifications", func(r chi.Router) {
			r.Method(http.MethodPost, "/", appHandler(s.sendNotification))
			r.Method(http.MethodGet, "/", appHandler(s.notificationJobs))
			r.Method(http.MethodGet, "/{job_id}", appHandler(s.notificationJob))
		})

		r.Method(http.MethodGet, "/metrics", appHandler(s.metrics))
	})
	return r
}
