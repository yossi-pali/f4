package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/12go/f4/internal/api/middleware"
)

// pageOverrideRepo defines the interface for admin page overrides.
type pageOverrideRepo interface {
	GetByURL(ctx context.Context, url string) (map[string]string, error)
	Set(ctx context.Context, url, param, value string) error
}

// AdminSearchHandler handles admin search form endpoints.
type AdminSearchHandler struct {
	pageOverrides pageOverrideRepo
}

func NewAdminSearchHandler(pageOverrides pageOverrideRepo) *AdminSearchHandler {
	return &AdminSearchHandler{pageOverrides: pageOverrides}
}

// ServeHTTPGet handles GET /admin/search/{fromPlaceID}/{toPlaceID}/{date}
// Returns admin form data including page overrides.
func (h *AdminSearchHandler) ServeHTTPGet(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if !agent.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	fromPlaceID := chi.URLParam(r, "fromPlaceID")
	toPlaceID := chi.URLParam(r, "toPlaceID")
	date := chi.URLParam(r, "date")

	pageURL := "/search/" + fromPlaceID + "/" + toPlaceID + "/" + date
	overrides, err := h.pageOverrides.GetByURL(r.Context(), pageURL)
	if err != nil {
		overrides = map[string]string{}
	}

	resp := map[string]any{
		"from_place_id":  fromPlaceID,
		"to_place_id":    toPlaceID,
		"date":           date,
		"page_overrides": overrides,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ServeHTTPPost handles POST /admin/search/{fromPlaceID}/{toPlaceID}
// Updates page meta overrides.
func (h *AdminSearchHandler) ServeHTTPPost(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if !agent.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	fromPlaceID := chi.URLParam(r, "fromPlaceID")
	toPlaceID := chi.URLParam(r, "toPlaceID")

	var body struct {
		Date      string            `json:"date"`
		Overrides map[string]string `json:"overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	pageURL := "/search/" + fromPlaceID + "/" + toPlaceID + "/" + body.Date

	for param, value := range body.Overrides {
		if err := h.pageOverrides.Set(r.Context(), pageURL, param, value); err != nil {
			http.Error(w, `{"error":"failed to save overrides"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
