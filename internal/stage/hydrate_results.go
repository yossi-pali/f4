package stage

import (
	"context"
	"fmt"
	"strings"

	"github.com/12go/f4/internal/domain"
)

// HydratedResults is the output of Stage 7.
type HydratedResults struct {
	Trips     []domain.TripResult
	Operators map[int]domain.Operator
	Stations  map[int]domain.Station
	Classes   map[int]domain.VehicleClass
	Filter    domain.SearchFilter
}

// HydrateResultsStage builds TripResult DTOs from raw trips and reference data.
type HydrateResultsStage struct{}

func NewHydrateResultsStage() *HydrateResultsStage { return &HydrateResultsStage{} }

func (s *HydrateResultsStage) Name() string { return "hydrate_results" }

func (s *HydrateResultsStage) Execute(_ context.Context, in EnrichedTrips) (HydratedResults, error) {
	out := HydratedResults{
		Operators: in.Operators,
		Stations:  in.Stations,
		Classes:   in.Classes,
		Filter:    in.Filter,
	}

	results := make([]domain.TripResult, 0, len(in.Trips))
	for _, raw := range in.Trips {
		tr := s.hydrateTrip(raw, in)
		results = append(results, tr)
	}
	out.Trips = results

	return out, nil
}

func (s *HydrateResultsStage) hydrateTrip(raw domain.RawTrip, in EnrichedTrips) domain.TripResult {
	tr := domain.TripResult{
		TripKey:       raw.TripKey,
		IsBookable:    raw.OpBookable && raw.Price.IsValid,
		HasValidPrice: raw.Price.IsValid,
		RankScore:     raw.RankScoreFormula,
		SpecialDeal:   raw.SpecialDealFlag,
		NewTrip:       raw.NewTripFlag,
		IsConnection:  raw.SetID != nil || raw.Departure2Time > 0,
	}

	// Build group key for merging duplicates
	tr.GroupKey = fmt.Sprintf("%d-%d-%d-%d",
		raw.DepStationID, raw.ArrStationID, raw.DepartureTime, raw.Duration)

	// Build primary segment
	seg := domain.Segment{
		FromStationID: raw.DepStationID,
		ToStationID:   raw.ArrStationID,
		Departure:     raw.Dep,
		Arrival:       raw.Arr,
		Duration:      raw.Duration,
		OperatorID:    raw.OperatorID,
		ClassID:       raw.ClassID,
		VehclassID:    raw.VehclassID,
		Type:          "route",
	}
	tr.Segments = []domain.Segment{seg}

	// Build travel option
	opt := domain.TravelOption{
		Price:           raw.Price,
		TripKey:         raw.TripKey,
		IntegrationCode: raw.IntegrationCode,
		DepartureTime:   raw.DepartureTime,
		Departure2Time:  raw.Departure2Time,
		Departure3Time:  raw.Departure3Time,
		UniqueKey: fmt.Sprintf("%s-%d-%d-%d",
			raw.TripKey, raw.DepartureTime, raw.Departure2Time, raw.Departure3Time),
	}
	if raw.Price.Avail > 0 {
		opt.AvailableSeats = raw.Price.Avail
	}
	tr.TravelOptions = []domain.TravelOption{opt}

	// Build tags
	tr.Tags = s.buildTags(raw)

	return tr
}

func (s *HydrateResultsStage) buildTags(raw domain.RawTrip) []string {
	var tags []string

	if raw.Amenities != "" {
		for _, a := range strings.Split(raw.Amenities, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				tags = append(tags, a)
			}
		}
	}
	if raw.TicketType != "" {
		tags = append(tags, "ticket:"+raw.TicketType)
	}
	if raw.BaggageFreeWeight > 0 {
		tags = append(tags, fmt.Sprintf("baggage:%dkg", raw.BaggageFreeWeight))
	}
	if raw.IsFRefundable {
		tags = append(tags, "refundable")
	}
	if raw.SpecialDealFlag {
		tags = append(tags, "special_deal")
	}
	if raw.NewTripFlag {
		tags = append(tags, "new")
	}

	return tags
}
