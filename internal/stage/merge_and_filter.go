package stage

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/pipeline"
)

// RecheckGroup represents one integration chunk that needs price rechecking.
// Each group produces one recheck URL, matching PHP Rechecker::getRecheckUrls.
type RecheckGroup struct {
	ChunkKey       string
	IntegrationID  int
	FromStationIDs []int // paired with ToStationIDs by index (unique pairs)
	ToStationIDs   []int
}

// PackRecheckEntry holds per-leg data for one BuyItem in a manual pack recheck.
// PHP Rechecker generates: headTripKey tripKey date (space-separated).
type PackRecheckEntry struct {
	HeadTripKey string // pack's master trip key (set's trip_key)
	TripKey     string // individual leg trip key
	Date        string // "YYYY-MM-DD" departure date of the leg
}

// PackRecheckGroup collects entries for one /searchpm URL, grouped by chunk key.
type PackRecheckGroup struct {
	Entries []PackRecheckEntry
}

// MergedResults is the output of Stage 8a.
type MergedResults struct {
	Trips               []domain.TripResult
	RecheckTripKeys     []string           // flat trip keys for event emission
	RecheckGroups       []RecheckGroup     // per-ChunkKey groups for URL generation (/searchr)
	PackRecheckGroups   []PackRecheckGroup // manual pack recheck groups (/searchpm)
	PresentIntegrations []string
	Operators           map[int]domain.Operator
	Stations            map[int]domain.Station
	Classes             map[int]domain.VehicleClass
	ToProvinceName      string
}

// MergeAndFilterStage merges duplicates, deduplicates travel options,
// filters invalid prices, and collects recheck groups.
type MergeAndFilterStage struct{}

func NewMergeAndFilterStage() *MergeAndFilterStage {
	return &MergeAndFilterStage{}
}

func (s *MergeAndFilterStage) Name() string { return "merge_and_filter" }

func (s *MergeAndFilterStage) Execute(ctx context.Context, in HydratedResults) (MergedResults, error) {
	out := MergedResults{
		Operators: in.Operators,
		Stations:  in.Stations,
		Classes:   in.Classes,
	}

	pc := pipeline.FromContext(ctx)
	filter := pc.Filter()
	preFilterRecheckEntries := pc.PreFilterRecheckEntries()
	pendingPackRechecks := pc.PendingPackRechecks()
	const stage = "merge_and_filter"

	// Province name pre-resolved in Stage 1 (resolve_places)
	out.ToProvinceName = filter.ToProvinceName

	if len(in.Trips) == 0 {
		out.Trips = []domain.TripResult{}
		return out, nil
	}

	t := pc.StartTimer(stage, "merge_loop")
	// Merge duplicate travel options by group key.
	// PHP TripResultApiV1Factory: when an existing trip is NOT bookable and a new
	// travel option IS bookable, the trip-level data (segments, params, transfer_id)
	// is replaced by the bookable option's data. This is the "winning option" logic.
	grouped := make(map[string]*domain.TripResult, len(in.Trips))
	var orderedKeys []string
	for i := range in.Trips {
		trip := &in.Trips[i]
		key := trip.GroupKey

		if existing, ok := grouped[key]; ok {
			existingBookable := tripHasBookableOption(existing)
			newBookable := tripHasBookableOption(trip)

			if !existingBookable && newBookable {
				// Winning option: replace trip-level data with bookable option's data,
				// but preserve ALL previously collected travel options.
				// PHP also loses the reason here (creates new trip object from bookable raw trip).
				prevOpts := existing.TravelOptions
				*existing = *trip
				existing.TravelOptions = append(prevOpts, trip.TravelOptions...)
			} else {
				// Normal merge: just append travel options
				existing.TravelOptions = append(existing.TravelOptions, trip.TravelOptions...)
				if trip.RankScore > existing.RankScore {
					existing.RankScore = trip.RankScore
				}
				// PHP line 127: adopt reason from subsequent trip if not yet set.
				// PHP uses "if ($rawTrip['reason_id'] && !$trip->params->reasonId)"
				if existing.ParamsReason == "" && trip.ParamsReason != "" {
					existing.ParamsReason = trip.ParamsReason
				}
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

	// Pack recheck grouping (PHP manualPacks collection → /searchpm URLs).
	// Grouped by chunk key like regular recheck, but entries contain per-leg
	// trip keys and dates instead of station pairs.
	type packRecheckGroupData struct {
		entries  []PackRecheckEntry
		entrySet map[string]struct{} // dedup key: "headTripKey tripKey"
	}
	packRecheckGroupMap := make(map[string]*packRecheckGroupData)
	var packRecheckGroupOrder []string

	// PHP originalCollection pattern (Pass B): merge pre-filter recheck entries
	// into the recheck groups. These entries come from ALL raw trips before
	// filtering (including connections that failed to assemble, meta trips, etc.).
	// PHP ChiefCook collects these BEFORE any filtering, then RecheckBuilder
	// merges them in Pass B.
	for _, entry := range preFilterRecheckEntries {
		chunkKey := buildPreFilterChunkKey(entry, in.ManualIntegrationID)
		gd, ok := recheckGroupMap[chunkKey]
		if !ok {
			intID := entry.IntegrationID
			if entry.IntegrationCode == "manual" && intID == 0 && in.ManualIntegrationID > 0 {
				intID = in.ManualIntegrationID
			}
			gd = &recheckGroupData{
				integrationID: intID,
				pairSet:       make(map[[2]int]struct{}),
			}
			recheckGroupMap[chunkKey] = gd
			recheckGroupOrder = append(recheckGroupOrder, chunkKey)
		}
		pair := [2]int{entry.DepStationID, entry.ArrStationID}
		if _, exists := gd.pairSet[pair]; !exists {
			gd.pairSet[pair] = struct{}{}
			gd.stationPairs = append(gd.stationPairs, pair)
		}
	}

	for _, key := range orderedKeys {
		trip := grouped[key]

		// Dedup travel options
		trip.TravelOptions = dedupTravelOptions(trip.TravelOptions)

		// PHP ChiefCook: aggregate stats from ALL travel options BEFORE filtering.
		// prepareMultiOptionTrip aggregates statistics; cookApiV1 aggregates params.
		aggregateMultiOptionTrip(trip)

		// For multi-option trips, reset params for re-aggregation matching PHP cookApiV1.
		// PHP: params are aggregated from all options with valid price (line 409),
		// BookingsLastMonth from filtered options only (line 443).
		isMulti := len(trip.TravelOptions) > 1
		if isMulti {
			trip.ParamsBookable = 0
			trip.ParamsIsBookable = 0
			trip.BookingsLastMonth = 0
			// PHP buildParams initialises min_price to Price{value:0, fxcode:…}.
			// Reset to zero-value (not nil) so we match PHP when all options have Total=0.
			fxCode := ""
			for _, o := range trip.TravelOptions {
				if o.Price.FXCode != "" {
					fxCode = o.Price.FXCode
					break
				}
			}
			trip.ParamsMinPrice = &domain.PriceSimple{Value: 0, FXCode: fxCode}
		}

		// PHP ChiefCook.cookApiV1: splitToRecheck=true by default.
		// Travel options without valid prices go to recheck only.
		// Valid-price options only appear in main results if:
		//   trip.showUnavailable (hide_days=0 or within window) OR bookable (Avail > 0) OR admin WithNonBookable.
		validOpts := make([]domain.TravelOption, 0, len(trip.TravelOptions))
		for _, opt := range trip.TravelOptions {
			if opt.IntegrationCode != "" {
				integrationSet[opt.IntegrationCode] = struct{}{}
			}

			// PHP cookApiV1 line 409: aggregate params from options with valid price > 0.
			// PHP comparison: if (minPrice->value < 0.05 || minPrice->value > option->price->value)
			if isMulti && opt.Price.IsValid && opt.Price.Total > 0 {
				if trip.ParamsMinPrice == nil || trip.ParamsMinPrice.Value < 0.05 || opt.Price.Total < trip.ParamsMinPrice.Value {
					trip.ParamsMinPrice = &domain.PriceSimple{Value: opt.Price.Total, FXCode: opt.Price.FXCode}
				}
				if opt.Bookable > 0 {
					if opt.Bookable > trip.ParamsBookable {
						trip.ParamsBookable = opt.Bookable
					}
					trip.ParamsIsBookable = 1
				}
			}

			if !opt.Price.IsValid {
				recheckKeySet[opt.TripKey] = struct{}{}

				// PHP RecheckBuilder.addToCollection: if trip.isPack() → manualPacks
				if opt.IsPack && len(opt.PackLegs) > 0 {
					pgd, ok := packRecheckGroupMap[opt.ChunkKey]
					if !ok {
						pgd = &packRecheckGroupData{
							entrySet: make(map[string]struct{}),
						}
						packRecheckGroupMap[opt.ChunkKey] = pgd
						packRecheckGroupOrder = append(packRecheckGroupOrder, opt.ChunkKey)
					}
					for _, leg := range opt.PackLegs {
						dedupKey := opt.HeadTripKey + " " + leg.TripKey
						if _, exists := pgd.entrySet[dedupKey]; exists {
							continue
						}
						pgd.entrySet[dedupKey] = struct{}{}
						pgd.entries = append(pgd.entries, PackRecheckEntry{
							HeadTripKey: opt.HeadTripKey,
							TripKey:     leg.TripKey,
							Date:        formatGodateUnix(leg.Godate)[:10], // "YYYY-MM-DD"
						})
					}
					continue
				}

				// Regular recheck: build group by ChunkKey (matching PHP RecheckBuilder)
				gd, ok := recheckGroupMap[opt.ChunkKey]
				if !ok {
					gd = &recheckGroupData{
						integrationID: opt.IntegrationID,
						pairSet:       make(map[[2]int]struct{}),
					}
					recheckGroupMap[opt.ChunkKey] = gd
					recheckGroupOrder = append(recheckGroupOrder, opt.ChunkKey)
				}
				for _, buy := range opt.Buy {
					pair := [2]int{buy.FromID, buy.ToID}
					if _, exists := gd.pairSet[pair]; !exists {
						gd.pairSet[pair] = struct{}{}
						gd.stationPairs = append(gd.stationPairs, pair)
					}
				}
				continue
			}
			// PHP: if ($trip->showUnavailable || $travelOption->bookable > 0)
			if !trip.ShowUnavailable && opt.Bookable <= 0 && !filter.WithNonBookable {
				continue
			}

			// PHP cookApiV1 line 443: BookingsLastMonth from filtered options only.
			if isMulti && opt.BookingsLastMonth > trip.BookingsLastMonth {
				trip.BookingsLastMonth = opt.BookingsLastMonth
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

	t.Stop()

	t = pc.StartTimer(stage, "recheck_groups")
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
	// Merge pending pack rechecks (multi-day packs that couldn't be assembled)
	// into the pack recheck groups.
	for _, pp := range pendingPackRechecks {
		pgd, ok := packRecheckGroupMap[pp.ChunkKey]
		if !ok {
			pgd = &packRecheckGroupData{
				entrySet: make(map[string]struct{}),
			}
			packRecheckGroupMap[pp.ChunkKey] = pgd
			packRecheckGroupOrder = append(packRecheckGroupOrder, pp.ChunkKey)
		}
		for _, leg := range pp.Legs {
			dedupKey := pp.HeadTripKey + " " + leg.TripKey
			if _, exists := pgd.entrySet[dedupKey]; exists {
				continue
			}
			pgd.entrySet[dedupKey] = struct{}{}
			pgd.entries = append(pgd.entries, PackRecheckEntry{
				HeadTripKey: pp.HeadTripKey,
				TripKey:     leg.TripKey,
				Date:        leg.Date,
			})
		}
	}

	// Build pack recheck groups
	for _, chunkKey := range packRecheckGroupOrder {
		pgd := packRecheckGroupMap[chunkKey]
		out.PackRecheckGroups = append(out.PackRecheckGroups, PackRecheckGroup{
			Entries: pgd.entries,
		})
	}

	for code := range integrationSet {
		out.PresentIntegrations = append(out.PresentIntegrations, code)
	}
	t.Stop()

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
	trip.IsBookable = false
	// NOTE: BookingsLastMonth is NOT reset here — it's reset and re-aggregated
	// in the Execute filtering loop from filtered options only (matching PHP cookApiV1 line 443).

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
		// NOTE: BookingsLastMonth aggregation moved to Execute filtering loop
		// (PHP cookApiV1 aggregates from filtered options only, not all options).
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

// tripHasBookableOption checks if any travel option in the trip is bookable.
func tripHasBookableOption(trip *domain.TripResult) bool {
	for _, o := range trip.TravelOptions {
		if o.IsBookable > 0 {
			return true
		}
	}
	return false
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

// buildPreFilterChunkKey computes the chunk key for a PreFilterRecheckEntry,
// matching the same logic as buildRecheckChunkKey in hydrate_results.go.
func buildPreFilterChunkKey(entry domain.PreFilterRecheckEntry, manualIntegrationID int) string {
	integrationID := entry.IntegrationID
	if entry.IntegrationCode == "manual" && integrationID == 0 && manualIntegrationID > 0 {
		integrationID = manualIntegrationID
	}

	chunkKey := entry.ChunkKeyRaw
	if entry.IntegrationCode == "manual" {
		chunkKey = "date"
	} else if entry.VehclassID == "train" && strings.Contains(entry.IntegrationCode, "easybook") {
		chunkKey = "vehclass_id,dep_station_id,arr_station_id"
	}

	fields := strings.Split(chunkKey, ",")
	values := []string{strconv.Itoa(integrationID)}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if field == "date" {
			loc, _ := time.LoadLocation("Asia/Bangkok")
			t := time.Unix(entry.Godate, 0).In(loc)
			values = append(values, t.Format("2006-01-02"))
			continue
		}
		values = append(values, getPreFilterField(entry, field))
	}
	return strings.Join(values, "-")
}

func getPreFilterField(entry domain.PreFilterRecheckEntry, field string) string {
	switch field {
	case "vehclass_id":
		return entry.VehclassID
	case "dep_station_id":
		return strconv.Itoa(entry.DepStationID)
	case "arr_station_id":
		return strconv.Itoa(entry.ArrStationID)
	case "operator_id":
		return strconv.Itoa(entry.OperatorID)
	case "class_id":
		return strconv.Itoa(entry.ClassID)
	case "official_id":
		return entry.OfficialID
	default:
		return "?"
	}
}
