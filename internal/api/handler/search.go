package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/12go/f4/internal/api/middleware"
	"github.com/12go/f4/internal/api/response"
	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/stage"
)

// SearchHandler handles GET /search/{fromPlaceID}/{toPlaceID}/{date}
type SearchHandler struct {
	pipeline *stage.SearchPipeline
}

func NewSearchHandler(pipeline *stage.SearchPipeline) *SearchHandler {
	return &SearchHandler{pipeline: pipeline}
}

func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fromPlaceID := chi.URLParam(r, "fromPlaceID")
	toPlaceID := chi.URLParam(r, "toPlaceID")
	dateStr := chi.URLParam(r, "date")

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, `{"error":"invalid date format, expected YYYY-MM-DD"}`, http.StatusBadRequest)
		return
	}

	params := parseSearchParams(r)
	agent := middleware.AgentFromContext(r.Context())

	result, err := h.pipeline.Execute(r.Context(), stage.SearchPipelineInput{
		FromPlaceID: fromPlaceID,
		ToPlaceID:   toPlaceID,
		Date:        date,
		Params:      params,
		Agent:       agent,
	})
	if err != nil {
		log.Printf("pipeline error: %v", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	v1 := response.FromDomain(result.Trips, result.Recheck, result.Stations, result.Operators, result.Classes, result.ProvinceName)
	json.NewEncoder(w).Encode(v1)
}

func parseSearchParams(r *http.Request) domain.SearchParams {
	q := r.URL.Query()
	return domain.SearchParams{
		Lang:            q.Get("l"),
		SeatsAdult:      queryIntAny(q, 1, "a", "seats"),
		SeatsChild:      queryInt(q, "c", 0),
		SeatsInfant:     queryInt(q, "i", 0),
		FXCode:          q.Get("fxcode"),
		Direct:          q.Get("direct") == "1",
		RecheckAmount:   queryInt(q, "recheck_amount", 0),
		IsRecheck:       q.Get("r") == "1",
		CartHash:        q.Get("cart_hash"),
		OutboundTripRef: q.Get("outbound_trip_ref"),
		OnlyPairs:       q.Get("only_pairs") == "1",
		VehclassID:      q.Get("vehclass_id"),
		IntegrationCode: q.Get("integration_code"),
		WithNonBookable: q.Get("with_non_bookable") == "1",
		ExtendedFormat:  q.Get("extended") == "1",
		Referer:         r.Header.Get("Referer"),
	}
}

func queryInt(q map[string][]string, key string, def int) int {
	vals, ok := q[key]
	if !ok || len(vals) == 0 {
		return def
	}
	v, err := strconv.Atoi(vals[0])
	if err != nil {
		return def
	}
	return v
}

// queryIntAny tries multiple keys in order, returning the first found.
func queryIntAny(q map[string][]string, def int, keys ...string) int {
	for _, key := range keys {
		if vals, ok := q[key]; ok && len(vals) > 0 {
			if v, err := strconv.Atoi(vals[0]); err == nil {
				return v
			}
		}
	}
	return def
}
