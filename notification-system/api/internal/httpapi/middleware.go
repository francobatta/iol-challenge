package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// Middleware here has the standard shape, func(http.Handler) http.Handler, so that
// cross-cutting steps such as logging or rate limiting can be chained around it.
//
// The authenticated app's ID travels in the request context for the benefit of those
// steps. Handlers do not read the context themselves: appHandler hands them the ID as
// a parameter, so a handler cannot be written that forgets to look for it.

type appIDKey struct{}

func appIDFrom(ctx context.Context) (appID string, ok bool) {
	appID, ok = ctx.Value(appIDKey{}).(string)
	return appID, ok
}

// authenticateApp rejects requests without a valid bearer token and records the ID of
// the app the token was issued to in the context of those it lets through.
func (s *server) authenticateApp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		appID, err := s.tokens.Verify(tok)
		if err != nil {
			writeError(w, r, errUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), appIDKey{}, appID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Admin-Key")
		if subtle.ConstantTimeCompare([]byte(key), []byte(s.adminKey)) != 1 {
			writeError(w, r, errUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// A handler serves a request and returns an error for writeError to report.
type handler func(w http.ResponseWriter, r *http.Request) error

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		writeError(w, r, err)
	}
}

// An appHandler is a handler for a request made by an app. It must be served behind
// authenticateApp, which supplies the appID.
type appHandler func(w http.ResponseWriter, r *http.Request, appID string) error

func (h appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDFrom(r.Context())
	if !ok {
		// A route was mounted outside authenticateApp. Fail the request rather than
		// run the handler on behalf of no app.
		writeError(w, r, errors.New("api: app route served without authentication"))
		return
	}
	if err := h(w, r, appID); err != nil {
		writeError(w, r, err)
	}
}
