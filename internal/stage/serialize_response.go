package stage

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/event"
	"github.com/12go/f4/internal/pipeline"
)

// SearchResponse is the final output of Stage 9 (the entire pipeline).
type SearchResponse struct {
	Trips          []domain.TripResult        `json:"trips"`
	Recheck        []string                   `json:"recheck"`
	Stations       map[int]domain.Station     `json:"stations,omitempty"`
	Operators      map[int]domain.Operator    `json:"operators,omitempty"`
	Classes        map[int]domain.VehicleClass `json:"classes,omitempty"`
	ProvinceName   string                     `json:"provinceName,omitempty"`
	StageTimes     map[string]time.Duration   `json:"-"`
}

// SerializeResponseStage builds the final API response and emits events.
type SerializeResponseStage struct {
	publisher      event.Publisher
	recheckBaseURL string
}

func NewSerializeResponseStage(publisher event.Publisher, recheckBaseURL string) *SerializeResponseStage {
	return &SerializeResponseStage{
		publisher:      publisher,
		recheckBaseURL: recheckBaseURL,
	}
}

func (s *SerializeResponseStage) Name() string { return "serialize_response" }

func (s *SerializeResponseStage) Execute(ctx context.Context, in FinalResults) (SearchResponse, error) {
	resp := SearchResponse{
		Trips:        in.Trips,
		Stations:     in.Stations,
		Operators:    in.Operators,
		Classes:      in.Classes,
		ProvinceName: in.ToProvinceName,
	}

	if resp.Trips == nil {
		resp.Trips = []domain.TripResult{}
	}

	// Build recheck URLs
	if len(in.RecheckTripKeys) > 0 {
		resp.Recheck = s.buildRecheckURLs(in)
	}
	if resp.Recheck == nil {
		resp.Recheck = []string{}
	}

	// Collect stage times from pipeline context
	if pc := pipeline.FromContext(ctx); pc != nil {
		resp.StageTimes = pc.StageTimes()
	}

	// Emit events (fire-and-forget, errors are non-fatal)
	s.emitEvents(ctx, in)

	return resp, nil
}

func (s *SerializeResponseStage) buildRecheckURLs(in FinalResults) []string {
	if s.recheckBaseURL == "" {
		return nil
	}

	// Group recheck trip keys by batches (max ~50 per URL to avoid URL length limits)
	const batchSize = 50
	var urls []string

	dateStr := in.Filter.Date.Format("2006-01-02")
	fromIDs := intsToString(in.Filter.FromStationIDs)
	toIDs := intsToString(in.Filter.ToStationIDs)

	for i := 0; i < len(in.RecheckTripKeys); i += batchSize {
		end := i + batchSize
		if end > len(in.RecheckTripKeys) {
			end = len(in.RecheckTripKeys)
		}
		batch := in.RecheckTripKeys[i:end]

		params := url.Values{}
		params.Set("l", in.Filter.Lang)
		params.Set("f", fromIDs)
		params.Set("t", toIDs)
		params.Set("d", dateStr)
		params.Set("a", fmt.Sprintf("%d", in.Filter.SeatsAdult))
		params.Set("c", fmt.Sprintf("%d", in.Filter.SeatsChild))
		params.Set("i", fmt.Sprintf("%d", in.Filter.SeatsInfant))
		params.Set("fxcode", in.Filter.FXCode)
		params.Set("keys", strings.Join(batch, ","))

		recheckURL := fmt.Sprintf("%s/recheck?%s", s.recheckBaseURL, params.Encode())
		urls = append(urls, recheckURL)
	}

	return urls
}

func (s *SerializeResponseStage) emitEvents(ctx context.Context, in FinalResults) {
	dateStr := in.Filter.Date.Format("2006-01-02")

	// search.completed event
	_ = s.publisher.Publish(ctx, domain.TopicSearchCompleted, domain.SearchCompletedEvent{
		FromPlaceID: in.Filter.FromPlaceID,
		ToPlaceID:   in.Filter.ToPlaceID,
		Date:        dateStr,
		ResultCount: len(in.Trips),
		AgentID:     in.Filter.AgentID,
		Timestamp:   time.Now(),
	})

	// search.needs_recheck event
	if len(in.RecheckTripKeys) > 0 {
		_ = s.publisher.Publish(ctx, domain.TopicSearchNeedsRecheck, domain.SearchNeedsRecheckEvent{
			TripKeys:       in.RecheckTripKeys,
			FromStationIDs: in.Filter.FromStationIDs,
			ToStationIDs:   in.Filter.ToStationIDs,
			Date:           dateStr,
			SeatsAdult:     in.Filter.SeatsAdult,
			SeatsChild:     in.Filter.SeatsChild,
			SeatsInfant:    in.Filter.SeatsInfant,
			FXCode:         in.Filter.FXCode,
			Lang:           in.Filter.Lang,
			AgentID:        in.Filter.AgentID,
		})
	}
}

func intsToString(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}
