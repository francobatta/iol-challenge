package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

func (s *server) createApp(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(w, r, &req); err != nil {
		return err
	}
	app, err := s.audiences.CreateApp(r.Context(), req.Name)
	if err != nil {
		return err
	}
	tok, err := s.tokens.Issue(app.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, struct {
		audience.App
		Token string `json:"token"`
	}{app, tok})
	return nil
}

func (s *server) registerUser(w http.ResponseWriter, r *http.Request, appID string) error {
	u, created, err := s.audiences.RegisterUser(r.Context(), appID, chi.URLParam(r, "user_id"))
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, u)
	return nil
}

func (s *server) user(w http.ResponseWriter, r *http.Request, appID string) error {
	u, err := s.audiences.User(r.Context(), appID, chi.URLParam(r, "user_id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, u)
	return nil
}

func (s *server) users(w http.ResponseWriter, r *http.Request, appID string) error {
	return servePage(w, r,
		func(u audience.User) string { return u.ID },
		func(p audience.Page) ([]audience.User, error) { return s.audiences.Users(r.Context(), appID, p) })
}

func (s *server) deleteUser(w http.ResponseWriter, r *http.Request, appID string) error {
	if err := s.audiences.DeleteUser(r.Context(), appID, chi.URLParam(r, "user_id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) createEndpoint(w http.ResponseWriter, r *http.Request, appID string) error {
	var req struct {
		Address  string           `json:"address"`
		Channel  audience.Channel `json:"channel"`
		Provider string           `json:"provider"`
	}
	if err := decode(w, r, &req); err != nil {
		return err
	}
	e, err := s.audiences.CreateEndpoint(r.Context(), appID, audience.Endpoint{
		UserID:   chi.URLParam(r, "user_id"),
		Address:  req.Address,
		Channel:  req.Channel,
		Provider: req.Provider,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, e)
	return nil
}

func (s *server) endpoint(w http.ResponseWriter, r *http.Request, appID string) error {
	e, err := s.audiences.Endpoint(r.Context(), appID, chi.URLParam(r, "endpoint_id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, e)
	return nil
}

func (s *server) endpoints(w http.ResponseWriter, r *http.Request, appID string) error {
	userID := chi.URLParam(r, "user_id")
	return servePage(w, r,
		func(e audience.Endpoint) string { return e.ID },
		func(p audience.Page) ([]audience.Endpoint, error) {
			return s.audiences.Endpoints(r.Context(), appID, userID, p)
		})
}

func (s *server) updateEndpoint(w http.ResponseWriter, r *http.Request, appID string) error {
	var update audience.EndpointUpdate
	if err := decode(w, r, &update); err != nil {
		return err
	}
	e, err := s.audiences.UpdateEndpoint(r.Context(), appID, chi.URLParam(r, "endpoint_id"), update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, e)
	return nil
}

func (s *server) deleteEndpoint(w http.ResponseWriter, r *http.Request, appID string) error {
	if err := s.audiences.DeleteEndpoint(r.Context(), appID, chi.URLParam(r, "endpoint_id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) createList(w http.ResponseWriter, r *http.Request, appID string) error {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decode(w, r, &req); err != nil {
		return err
	}
	l, err := s.audiences.CreateList(r.Context(), appID, audience.List{Name: req.Name, Description: req.Description})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, l)
	return nil
}

func (s *server) list(w http.ResponseWriter, r *http.Request, appID string) error {
	l, err := s.audiences.List(r.Context(), appID, chi.URLParam(r, "list_id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, l)
	return nil
}

func (s *server) lists(w http.ResponseWriter, r *http.Request, appID string) error {
	return servePage(w, r,
		func(l audience.List) string { return l.ID },
		func(p audience.Page) ([]audience.List, error) { return s.audiences.Lists(r.Context(), appID, p) })
}

func (s *server) updateList(w http.ResponseWriter, r *http.Request, appID string) error {
	var update audience.ListUpdate
	if err := decode(w, r, &update); err != nil {
		return err
	}
	l, err := s.audiences.UpdateList(r.Context(), appID, chi.URLParam(r, "list_id"), update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, l)
	return nil
}

func (s *server) deleteList(w http.ResponseWriter, r *http.Request, appID string) error {
	if err := s.audiences.DeleteList(r.Context(), appID, chi.URLParam(r, "list_id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) addMember(w http.ResponseWriter, r *http.Request, appID string) error {
	userIDs := []string{chi.URLParam(r, "user_id")}
	if err := s.audiences.AddMembers(r.Context(), appID, chi.URLParam(r, "list_id"), userIDs); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) addMembers(w http.ResponseWriter, r *http.Request, appID string) error {
	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := decode(w, r, &req); err != nil {
		return err
	}
	if err := s.audiences.AddMembers(r.Context(), appID, chi.URLParam(r, "list_id"), req.UserIDs); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) removeMember(w http.ResponseWriter, r *http.Request, appID string) error {
	err := s.audiences.RemoveMember(r.Context(), appID, chi.URLParam(r, "list_id"), chi.URLParam(r, "user_id"))
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) members(w http.ResponseWriter, r *http.Request, appID string) error {
	listID := chi.URLParam(r, "list_id")
	return servePage(w, r,
		func(m audience.Member) string { return m.UserID },
		func(p audience.Page) ([]audience.Member, error) {
			return s.audiences.Members(r.Context(), appID, listID, p)
		})
}
