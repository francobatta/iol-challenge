package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight"
	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
	maxBodyBytes    = 1 << 20
	// maxImportBytes bounds the one body that is streamed instead of decoded whole.
	maxImportBytes = 64 << 20
)

var errUnauthorized = errors.New("unauthorized")

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
	case errors.Is(err, notify.ErrQuotaExceeded):
		status, code, message = http.StatusTooManyRequests, "quota_exceeded", err.Error()
	case errors.Is(err, insight.ErrUnavailable):
		// Logged too: it is the operator who can do something about it.
		slog.WarnContext(r.Context(), "Metrics unavailable", "err", err)
		status, code, message = http.StatusServiceUnavailable, "unavailable", "metrics are unavailable"
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
