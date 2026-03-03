package stage

import (
	"context"
	"math"
	"sort"

	"github.com/12go/f4/internal/domain"
)

// RecheckGroup represents one integration chunk that needs price rechecking.
// Each group produces one recheck URL, matching PHP Rechecker::getRecheckUrls.
type RecheckGroup struct {
	ChunkKey       string
	IntegrationID  int
	FromStationIDs []int // paired with ToStationIDs by index (unique pairs)
	ToStationIDs   []int
}

// FinalResults is the output of Stage 8.
type FinalResults struct {
	Trips               []domain.TripResult
	RecheckTripKeys     []string       // flat trip keys for event emission
	RecheckGroups       []RecheckGroup // per-ChunkKey groups for URL generation
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

	// Recheck grouping by ChunkKey (matching PHP RecheckBuilder)
	type recheckGroupData struct {
		integrationID int
		stationPairs  [][2]int // unique (from, to) pairs in insertion order
		pairSet       map[[2]int]struct{}
	}
	recheckGroupMap := make(map[string]*recheckGroupData)
	var recheckGroupOrder []string // insertion order of ChunkKeys

	for _, key := range orderedKeys {
		trip := grouped[key]

		// Dedup travel options
		trip.TravelOptions = dedupTravelOptions(trip.TravelOptions)

		// PHP ChiefCook: aggregate stats from ALL travel options BEFORE filtering.
		// prepareMultiOptionTrip aggregates statistics; cookApiV1 aggregates params.
		aggregateMultiOptionTrip(trip)

		// PHP ChiefCook.cookApiV1: splitToRecheck=true by default.
		// Travel options without valid prices go to recheck only.
		// Valid-price options only appear in main results if:
		//   trip.showUnavailable (hide_days=0 or within window) OR bookable (Avail > 0) OR admin WithNonBookable.
		validOpts := make([]domain.TravelOption, 0, len(trip.TravelOptions))
		for _, opt := range trip.TravelOptions {
			if opt.IntegrationCode != "" {
				integrationSet[opt.IntegrationCode] = struct{}{}
			}
			if !opt.Price.IsValid {
				recheckKeySet[opt.TripKey] = struct{}{}
				// Build recheck group by ChunkKey (matching PHP RecheckBuilder grouping)
				gd, ok := recheckGroupMap[opt.ChunkKey]
				if !ok {
					gd = &recheckGroupData{
						integrationID: opt.IntegrationID,
						pairSet:       make(map[[2]int]struct{}),
					}
					recheckGroupMap[opt.ChunkKey] = gd
					recheckGroupOrder = append(recheckGroupOrder, opt.ChunkKey)
				}
				pair := [2]int{opt.FromStationID, opt.ToStationID}
				if _, exists := gd.pairSet[pair]; !exists {
					gd.pairSet[pair] = struct{}{}
					gd.stationPairs = append(gd.stationPairs, pair)
				}
				continue
			}
			// PHP: if ($trip->showUnavailable || $travelOption->bookable > 0)
			if !trip.ShowUnavailable && opt.Bookable <= 0 && !in.Filter.WithNonBookable {
				continue
			}
			validOpts = append(validOpts, opt)
		}

		// If no travel options pass the filter, skip trip from main results.
		if len(validOpts) == 0 {
			continue
		}
		trip.TravelOptions = validOpts
		trip.HasValidPrice = true

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
	for _, chunkKey := range recheckGroupOrder {
		gd := recheckGroupMap[chunkKey]
		g := RecheckGroup{
			ChunkKey:       chunkKey,
			IntegrationID:  gd.integrationID,
			FromStationIDs: make([]int, len(gd.stationPairs)),
			ToStationIDs:   make([]int, len(gd.stationPairs)),
		}
		for i, pair := range gd.stationPairs {
			g.FromStationIDs[i] = pair[0]
			g.ToStationIDs[i] = pair[1]
		}
		out.RecheckGroups = append(out.RecheckGroups, g)
	}
	for code := range integrationSet {
		out.PresentIntegrations = append(out.PresentIntegrations, code)
	}

	return out, nil
}

// aggregateMultiOptionTrip recalculates trip-level stats from all travel options,
// matching PHP ChiefCook::prepareMultiOptionTrip + cookApiV1 aggregation.
func aggregateMultiOptionTrip(trip *domain.TripResult) {
	if len(trip.TravelOptions) <= 1 {
		return
	}

	// Reset aggregated fields (PHP prepareMultiOptionTrip resets then re-aggregates)
	trip.ParamsRatingCount = nil
	trip.ParamsMinRating = nil
	trip.ScoreSorting = 0
	trip.SalesSorting = 0
	trip.BookingsLastMonth = 0
	trip.IsBookable = false

	var totalBookings30d, totalBookings30dSolo int
	var paramsRatingCount int

	for _, opt := range trip.TravelOptions {
		// PHP cookApiV1: params.ratingCount += travelOption.ratingCount (all options)
		if opt.RatingCount != nil {
			paramsRatingCount += *opt.RatingCount
		}
		// PHP cookApiV1: params.minRating = min(travelOption.rating) (all options)
		if opt.Rating != nil {
			if trip.ParamsMinRating == nil || *opt.Rating < *trip.ParamsMinRating {
				r := *opt.Rating
				trip.ParamsMinRating = &r
			}
		}
		// PHP addTripStatistics: rankScore = max
		if opt.ScoreSortingRaw > trip.ScoreSorting {
			trip.ScoreSorting = opt.ScoreSortingRaw
		}
		// PHP addTripStatistics: salesSorting = max (AB test OFF)
		// PHP uses $travelOption->rankSales (unconditional calculateRankSales),
		// NOT the zeroed-out travel option sales_sorting.
		optSales := rankSalesFromBookings(float64(opt.Bookings30d))
		if optSales > trip.SalesSorting {
			trip.SalesSorting = optSales
		}
		// PHP addTripStatistics: bookingsLastMonth = max
		if opt.BookingsLastMonth > trip.BookingsLastMonth {
			trip.BookingsLastMonth = opt.BookingsLastMonth
		}
		// PHP addTripStatistics: bookings30d += (sum)
		totalBookings30d += opt.Bookings30d
		totalBookings30dSolo += opt.Bookings30dSolo
		// PHP prepareMultiOptionTrip: isBookable = isBookable || option.isBookable
		if opt.IsBookable > 0 {
			trip.IsBookable = true
		}
	}

	if paramsRatingCount > 0 {
		trip.ParamsRatingCount = &paramsRatingCount
	}
	// PHP: isSoloTraveler from aggregated bookings (soloTravelerMinBookings30d = 30)
	trip.IsSoloTraveler = totalBookings30d >= 30 &&
		float64(totalBookings30dSolo)/float64(totalBookings30d)*100 >= 30
}

// rankSalesFromBookings matches PHP calculateRankSales (RANK_SCORE_SALES_REAL_MULTIPLIER = 40).
func rankSalesFromBookings(bookings30d float64) float64 {
	if bookings30d >= 1.0 {
		return math.Log(bookings30d * 40)
	}
	return 0
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
