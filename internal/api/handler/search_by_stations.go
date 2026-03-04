package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/12go/f4/internal/api/middleware"
	"github.com/12go/f4/internal/api/response"
	"github.com/12go/f4/internal/stage"
)

// SearchByStationsHandler handles GET /searchByStations/{fromStations}/{toStations}/{date}
type SearchByStationsHandler struct {
	pipeline *stage.SearchPipeline
	logger   *zap.Logger
}

func NewSearchByStationsHandler(pipeline *stage.SearchPipeline, logger *zap.Logger) *SearchByStationsHandler {
	return &SearchByStationsHandler{pipeline: pipeline, logger: logger}
}

func (h *SearchByStationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	fromStr := chi.URLParam(r, "fromStations")
	toStr := chi.URLParam(r, "toStations")
	dateStr := chi.URLParam(r, "date")

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, `{"error":"invalid date format, expected YYYY-MM-DD"}`, http.StatusBadRequest)
		return
	}

	fromIDs, err := parseDashSeparatedInts(fromStr)
	if err != nil || len(fromIDs) == 0 {
		http.Error(w, `{"error":"invalid fromStations"}`, http.StatusBadRequest)
		return
	}

	toIDs, err := parseDashSeparatedInts(toStr)
	if err != nil || len(toIDs) == 0 {
		http.Error(w, `{"error":"invalid toStations"}`, http.StatusBadRequest)
		return
	}

	params := parseSearchParams(r)
	agent := middleware.AgentFromContext(r.Context())

	result, err := h.pipeline.Execute(r.Context(), stage.SearchPipelineInput{
		FromStations: fromIDs,
		ToStations:   toIDs,
		Date:         date,
		Params:       params,
		Agent:        agent,
	})
	if err != nil {
		h.logger.Error("pipeline error", zap.Error(err))
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	elapsed := time.Since(start)
	writeStageTimingHeaders(w, result.StageTimes, elapsed)

	h.logger.Info("search_by_stations",
		zap.String("from", fromStr),
		zap.String("to", toStr),
		zap.String("date", dateStr),
		zap.Int("trips", len(result.Trips)),
		zap.Duration("total", elapsed),
		zap.Any("stages", result.StageTimes),
	)

	w.Header().Set("Content-Type", "application/json")
	v1 := response.FromDomain(result.Trips, result.Recheck, result.Stations, result.Operators, result.Classes, result.ProvinceName)
	json.NewEncoder(w).Encode(v1)
}

func parseDashSeparatedInts(s string) ([]int, error) {
	parts := strings.Split(s, "-")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
