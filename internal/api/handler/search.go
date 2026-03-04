package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/12go/f4/internal/api/middleware"
	"github.com/12go/f4/internal/api/response"
	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/stage"
)

// SearchHandler handles GET /search/{fromPlaceID}/{toPlaceID}/{date}
type SearchHandler struct {
	pipeline *stage.SearchPipeline
	logger   *zap.Logger
}

func NewSearchHandler(pipeline *stage.SearchPipeline, logger *zap.Logger) *SearchHandler {
	return &SearchHandler{pipeline: pipeline, logger: logger}
}

func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
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
		h.logger.Error("pipeline error", zap.Error(err))
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	elapsed := time.Since(start)
	writeStageTimingHeaders(w, result.StageTimes, elapsed)

	h.logger.Info("search",
		zap.String("from", fromPlaceID),
		zap.String("to", toPlaceID),
		zap.String("date", dateStr),
		zap.Int("trips", len(result.Trips)),
		zap.Duration("total", elapsed),
		zap.Any("stages", result.StageTimes),
	)

	w.Header().Set("Content-Type", "application/json")
	v1 := response.FromDomain(result.Trips, result.Recheck, result.Stations, result.Operators, result.Classes, result.ProvinceName)
	json.NewEncoder(w).Encode(v1)
}

// writeStageTimingHeaders sets X-Stage-* and X-Total-Time headers.
func writeStageTimingHeaders(w http.ResponseWriter, stages map[string]time.Duration, total time.Duration) {
	keys := make([]string, 0, len(stages))
	for k := range stages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w.Header().Set(fmt.Sprintf("X-Stage-%s", k), fmt.Sprintf("%dms", stages[k].Milliseconds()))
	}
	w.Header().Set("X-Total-Time", fmt.Sprintf("%dms", total.Milliseconds()))
}

func parseSearchParams(r *http.Request) domain.SearchParams {
	q := r.URL.Query()
	return domain.SearchParams{
		Lang:            q.Get("l"),
		SeatsAdult:      queryIntAny(q, 1, "seats_adult", "seats"),
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
		SearchURL:       buildSearchURL(r),
		VisitorID:       q.Get("visitorId"),
	}
}

// buildSearchURL reconstructs the request URL without query string,
// matching PHP: $this->request->getUri() with query stripped.
func buildSearchURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	return scheme + "://" + host + r.URL.Path
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
