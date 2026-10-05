// Package api serves the audience REST API over HTTP.
//
// Every route but app creation is called by an app, which identifies itself with a
// bearer token. The app's ID always comes from that token, never from the URL or body.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/token"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
	maxBodyBytes    = 1 << 20
)

var errUnauthorized = errors.New("unauthorized")

type server struct {
	svc      *audience.Service
	tokens   *token.Signer
	adminKey string
}

// NewHandler returns the handler for the whole API. Requests to create an app must
// carry adminKey in the X-Admin-Key header; all other requests must carry a token
// issued by tokens.
func NewHandler(svc *audience.Service, tokens *token.Signer, adminKey string) http.Handler {
	s := &server{svc: svc, tokens: tokens, adminKey: adminKey}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/apps", s.asAdmin(s.createApp))

	mux.HandleFunc("PUT /v1/users/{user_id}", s.asApp(s.registerUser))
	mux.HandleFunc("GET /v1/users/{user_id}", s.asApp(s.user))
	mux.HandleFunc("GET /v1/users", s.asApp(s.users))
	mux.HandleFunc("DELETE /v1/users/{user_id}", s.asApp(s.deleteUser))

	mux.HandleFunc("POST /v1/users/{user_id}/endpoints", s.asApp(s.createEndpoint))
	mux.HandleFunc("GET /v1/users/{user_id}/endpoints", s.asApp(s.endpoints))
	mux.HandleFunc("GET /v1/endpoints/{endpoint_id}", s.asApp(s.endpoint))
	mux.HandleFunc("PATCH /v1/endpoints/{endpoint_id}", s.asApp(s.updateEndpoint))
	mux.HandleFunc("DELETE /v1/endpoints/{endpoint_id}", s.asApp(s.deleteEndpoint))

	mux.HandleFunc("POST /v1/lists", s.asApp(s.createList))
	mux.HandleFunc("GET /v1/lists", s.asApp(s.lists))
	mux.HandleFunc("GET /v1/lists/{list_id}", s.asApp(s.list))
	mux.HandleFunc("PATCH /v1/lists/{list_id}", s.asApp(s.updateList))
	mux.HandleFunc("DELETE /v1/lists/{list_id}", s.asApp(s.deleteList))

	mux.HandleFunc("PUT /v1/lists/{list_id}/members/{user_id}", s.asApp(s.addMember))
	mux.HandleFunc("DELETE /v1/lists/{list_id}/members/{user_id}", s.asApp(s.removeMember))
	mux.HandleFunc("GET /v1/lists/{list_id}/members", s.asApp(s.members))
	mux.HandleFunc("POST /v1/lists/{list_id}/members", s.asApp(s.addMembers))

	return mux
}

// asAdmin runs h only for requests that carry the admin key.
func (s *server) asAdmin(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Admin-Key")
		if subtle.ConstantTimeCompare([]byte(key), []byte(s.adminKey)) != 1 {
			writeError(w, r, errUnauthorized)
			return
		}
		if err := h(w, r); err != nil {
			writeError(w, r, err)
		}
	}
}

// asApp runs h with the ID of the app that the request's bearer token was issued to.
func (s *server) asApp(h func(w http.ResponseWriter, r *http.Request, appID string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		appID, err := s.tokens.Verify(tok)
		if err != nil {
			writeError(w, r, errUnauthorized)
			return
		}
		if err := h(w, r, appID); err != nil {
			writeError(w, r, err)
		}
	}
}

// decode reads the JSON request body into dst.
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: request body: %v", audience.ErrInvalid, err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status is already sent, so a failure here (the client went away) cannot be
	// reported to anyone.
	_ = json.NewEncoder(w).Encode(body)
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError sends the response for err. Errors that are not the caller's fault are
// logged and reported without detail.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusInternalServerError, "internal", "internal error"
	switch {
	case errors.Is(err, errUnauthorized):
		status, code, message = http.StatusUnauthorized, "unauthorized", "missing or invalid credentials"
	case errors.Is(err, audience.ErrInvalid):
		status, code, message = http.StatusBadRequest, "invalid_request", err.Error()
	case errors.Is(err, audience.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, audience.ErrConflict):
		status, code, message = http.StatusConflict, "conflict", err.Error()
	default:
		slog.ErrorContext(r.Context(), "Request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

type pageBody[T any] struct {
	Items     []T    `json:"items"`
	NextAfter string `json:"next_after,omitempty"`
}

// servePage answers a request for one page of a collection. fetch loads the items
// selected by a Page and id returns the ID the collection is ordered by.
func servePage[T any](w http.ResponseWriter, r *http.Request, id func(T) string, fetch func(audience.Page) ([]T, error)) error {
	query := r.URL.Query()
	limit := defaultPageSize
	if raw := query.Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxPageSize {
			return fmt.Errorf("%w: limit must be a number from 1 to %d", audience.ErrInvalid, maxPageSize)
		}
	}

	// Fetching one item more than asked for tells us whether another page follows.
	items, err := fetch(audience.Page{After: query.Get("after"), Limit: limit + 1})
	if err != nil {
		return err
	}
	body := pageBody[T]{Items: items}
	if len(items) > limit {
		body.Items = items[:limit]
		body.NextAfter = id(items[limit-1])
	}
	if body.Items == nil {
		body.Items = []T{} // encode as [] rather than null
	}
	writeJSON(w, http.StatusOK, body)
	return nil
}
