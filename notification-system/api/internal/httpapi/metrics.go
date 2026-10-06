package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

// defaultMetricsWindow is how far back a snapshot looks when the request does not say.
const defaultMetricsWindow = 15 * time.Minute

func (s *server) metrics(w http.ResponseWriter, r *http.Request, appID string) error {
	window := defaultMetricsWindow
	if raw := r.URL.Query().Get("range"); raw != "" {
		var err error
		window, err = time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: range must be a duration such as 15m or 1h", audience.ErrInvalid)
		}
	}
	snapshot, err := s.insights.Snapshot(r.Context(), appID, window)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, snapshot)
	return nil
}
