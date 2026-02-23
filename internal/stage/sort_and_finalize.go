package stage

import (
	"context"
	"sort"

	"github.com/12go/f4/internal/domain"
)

// FinalResults is the output of Stage 8.
type FinalResults struct {
	Trips               []domain.TripResult
	RecheckTripKeys     []string
	PresentIntegrations []string
	Operators           map[int]domain.Operator
	Stations            map[int]domain.Station
	Classes             map[int]domain.VehicleClass
	Filter              domain.SearchFilter
	ToProvinceName      string
}

// SortAndFinalizeStage merges duplicates, filters invalid, and sorts results.
type SortAndFinalizeStage struct {
	stationRepo interface {
		GetParentProvinceName(ctx context.Context, placeID string) string
	}
}

func NewSortAndFinalizeStage(stationRepo interface {
	GetParentProvinceName(ctx context.Context, placeID string) string
}) *SortAndFinalizeStage {
	return &SortAndFinalizeStage{stationRepo: stationRepo}
}

func (s *SortAndFinalizeStage) Name() string { return "sort_and_finalize" }

func (s *SortAndFinalizeStage) Execute(ctx context.Context, in HydratedResults) (FinalResults, error) {
	out := FinalResults{
		Operators: in.Operators,
		Stations:  in.Stations,
		Classes:   in.Classes,
		Filter:    in.Filter,
	}

	// Get to province name for response
	if in.Filter.ToPlaceID != domain.UnknownPlace {
		out.ToProvinceName = s.stationRepo.GetParentProvinceName(ctx, in.Filter.ToPlaceID)
	}

	if len(in.Trips) == 0 {
		out.Trips = []domain.TripResult{}
		return out, nil
	}

	// Merge duplicate travel options by group key
	grouped := make(map[string]*domain.TripResult, len(in.Trips))
	var orderedKeys []string
	for i := range in.Trips {
		trip := &in.Trips[i]
		key := trip.GroupKey

		if existing, ok := grouped[key]; ok {
			// Merge travel options
			existing.TravelOptions = append(existing.TravelOptions, trip.TravelOptions...)
			// Keep best rank score
			if trip.RankScore > existing.RankScore {
				existing.RankScore = trip.RankScore
			}
		} else {
			t := *trip
			grouped[key] = &t
			orderedKeys = append(orderedKeys, key)
		}
	}

	// Dedup travel options within each group by UniqueKey
	trips := make([]domain.TripResult, 0, len(grouped))
	integrationSet := make(map[string]struct{})
	recheckKeySet := make(map[string]struct{})

	for _, key := range orderedKeys {
		trip := grouped[key]

		// Dedup travel options
		trip.TravelOptions = dedupTravelOptions(trip.TravelOptions)

		// Filter non-bookable (unless admin)
		if !in.Filter.WithNonBookable && !trip.IsBookable {
			continue
		}

		// Filter invalid prices (unless admin)
		if !in.Filter.WithAdminLinks && !trip.HasValidPrice {
			continue
		}

		// Collect integrations and recheck keys
		for _, opt := range trip.TravelOptions {
			if opt.IntegrationCode != "" {
				integrationSet[opt.IntegrationCode] = struct{}{}
			}
			if !opt.Price.IsValid || opt.Price.PriceLevel != domain.PriceExact {
				recheckKeySet[opt.TripKey] = struct{}{}
			}
		}

		trips = append(trips, *trip)
	}

	// Sort: bookable first → valid price → special deals → rank score
	sort.SliceStable(trips, func(i, j int) bool {
		a, b := trips[i], trips[j]
		if a.IsBookable != b.IsBookable {
			return a.IsBookable
		}
		if a.HasValidPrice != b.HasValidPrice {
			return a.HasValidPrice
		}
		if a.SpecialDeal != b.SpecialDeal {
			return a.SpecialDeal
		}
		return a.RankScore > b.RankScore
	})

	out.Trips = trips
	for key := range recheckKeySet {
		out.RecheckTripKeys = append(out.RecheckTripKeys, key)
	}
	for code := range integrationSet {
		out.PresentIntegrations = append(out.PresentIntegrations, code)
	}

	return out, nil
}

func dedupTravelOptions(opts []domain.TravelOption) []domain.TravelOption {
	seen := make(map[string]struct{}, len(opts))
	result := make([]domain.TravelOption, 0, len(opts))
	for _, o := range opts {
		if _, ok := seen[o.UniqueKey]; ok {
			continue
		}
		seen[o.UniqueKey] = struct{}{}
		result = append(result, o)
	}
	return result
}
