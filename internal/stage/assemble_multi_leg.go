package stage

import (
	"context"
	"fmt"

	"github.com/12go/f4/internal/db"
	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/repository"
)

// AssembleMultiLegInput is the input for Stage 5a.
type AssembleMultiLegInput struct {
	DirectTrips              []domain.RawTrip
	ConnectionIDs            []int
	ConnectionCompositeRows  map[int][]CompositeRow // set_id → composite rows with (godate, dep2) tuples
	Filter                   domain.SearchFilter
	SearchParams             repository.SearchParams // price params needed for fetching missing connection legs
}

// MultiLegTrips is the output of Stage 5a.
type MultiLegTrips struct {
	Connections         []domain.RawTrip
	Autopacks           []domain.RawTrip
	PendingPackRechecks []domain.PendingPackRecheck // multi-day packs that couldn't be assembled
}

// AssembleMultiLegStage builds connections from trip_pool4_set and autopacks.
type AssembleMultiLegStage struct {
	tripPoolSetRepo *repository.TripPoolSetRepo
	tripPoolRepo    *repository.TripPoolRepo
	autopackRepo    *repository.AutopackRepo
	regionResolver  db.RegionResolver
}

func NewAssembleMultiLegStage(
	tripPoolSetRepo *repository.TripPoolSetRepo,
	tripPoolRepo *repository.TripPoolRepo,
	autopackRepo *repository.AutopackRepo,
	regionResolver db.RegionResolver,
) *AssembleMultiLegStage {
	return &AssembleMultiLegStage{
		tripPoolSetRepo: tripPoolSetRepo,
		tripPoolRepo:    tripPoolRepo,
		autopackRepo:    autopackRepo,
		regionResolver:  regionResolver,
	}
}

func (s *AssembleMultiLegStage) Name() string { return "assemble_multi_leg" }

func (s *AssembleMultiLegStage) Execute(ctx context.Context, in AssembleMultiLegInput) (MultiLegTrips, error) {
	var out MultiLegTrips

	// Build connections from trip_pool4_set
	if len(in.ConnectionIDs) > 0 {
		conns, pendingPacks, err := s.buildConnections(ctx, in)
		if err != nil {
			return out, err
		}
		out.Connections = conns
		out.PendingPackRechecks = pendingPacks
	}

	// Build autopacks
	if in.Filter.WithAutopacks && in.Filter.FromPlaceID != domain.UnknownPlace && in.Filter.ToPlaceID != domain.UnknownPlace {
		packs, err := s.buildAutopacks(ctx, in)
		if err != nil {
			return out, err
		}
		out.Autopacks = packs
	}

	return out, nil
}

func (s *AssembleMultiLegStage) buildConnections(ctx context.Context, in AssembleMultiLegInput) ([]domain.RawTrip, []domain.PendingPackRecheck, error) {
	// Determine region from first departure station
	region := db.DefaultRegion
	if len(in.Filter.FromStationIDs) > 0 {
		region = s.regionResolver.ResolveByStationID(in.Filter.FromStationIDs[0])
	}

	sets, err := s.tripPoolSetRepo.FindBySetIDs(ctx, region, in.ConnectionIDs)
	if err != nil {
		return nil, nil, err
	}

	// Build a trip lookup by trip key for matching legs
	tripByKey := make(map[string]domain.RawTrip, len(in.DirectTrips))
	for _, t := range in.DirectTrips {
		tripByKey[t.TripKey] = t
	}

	// PHP's PackAndConnectionSearch fetches connection legs separately by trip_key
	// from trip_pool4. The main search query uses route_place constraints, so legs
	// whose station pairs don't match the route (e.g., Don Mueang→Mo Chit for a
	// Chiang Mai→Bangkok connection) aren't in DirectTrips. Collect missing trip_keys
	// and fetch them in a second pass.
	missingKeys := make(map[string]struct{})
	// Multi-day legs need fetching with an adjusted godate. Group by day offset.
	multiDayKeys := make(map[int]map[string]struct{}) // dayOffset → set of trip_keys
	for _, set := range sets {
		if _, ok := tripByKey[set.Trip1Key]; !ok {
			missingKeys[set.Trip1Key] = struct{}{}
		}
		if _, ok := tripByKey[set.Trip2Key]; !ok {
			if set.Trip2Day == 0 {
				missingKeys[set.Trip2Key] = struct{}{}
			} else {
				if multiDayKeys[set.Trip2Day] == nil {
					multiDayKeys[set.Trip2Day] = make(map[string]struct{})
				}
				multiDayKeys[set.Trip2Day][set.Trip2Key] = struct{}{}
			}
		}
		if set.Trip3Key != nil && *set.Trip3Key != "" {
			if _, ok := tripByKey[*set.Trip3Key]; !ok {
				if set.Trip3Day == 0 {
					missingKeys[*set.Trip3Key] = struct{}{}
				} else {
					if multiDayKeys[set.Trip3Day] == nil {
						multiDayKeys[set.Trip3Day] = make(map[string]struct{})
					}
					multiDayKeys[set.Trip3Day][*set.Trip3Key] = struct{}{}
				}
			}
		}
	}
	if len(missingKeys) > 0 {
		keys := make([]string, 0, len(missingKeys))
		for k := range missingKeys {
			keys = append(keys, k)
		}
		fetched, err := s.tripPoolRepo.FindByTripKeys(ctx, region, keys, in.SearchParams)
		if err == nil {
			for _, t := range fetched {
				if _, exists := tripByKey[t.TripKey]; !exists {
					tripByKey[t.TripKey] = t
				}
			}
		}
		// Non-fatal: if fetch fails, continue with what we have
	}

	// Fetch multi-day legs with adjusted godate.
	// PHP PackAndConnectionSearch adjusts godate by trip2_day/trip3_day.
	for dayOffset, keySet := range multiDayKeys {
		keys := make([]string, 0, len(keySet))
		for k := range keySet {
			keys = append(keys, k)
		}
		adjustedDate := in.Filter.Date.AddDate(0, 0, dayOffset)
		adjustedParams := in.SearchParams
		adjustedParams.GodateString = adjustedDate.Format("2006-01-02")
		fetched, err := s.tripPoolRepo.FindByTripKeys(ctx, region, keys, adjustedParams)
		if err == nil {
			for _, t := range fetched {
				if _, exists := tripByKey[t.TripKey]; !exists {
					tripByKey[t.TripKey] = t
				}
			}
		}
	}

	searchDate := in.Filter.Date.Format("2006-01-02")

	var connections []domain.RawTrip
	var pendingPacks []domain.PendingPackRecheck
	for _, set := range sets {
		// Find leg trips in the direct trips
		leg1, ok1 := tripByKey[set.Trip1Key]
		leg2, ok2 := tripByKey[set.Trip2Key]
		if !ok1 || !ok2 {
			// For manual packs (pack_id > 0) that fail to assemble because
			// the second leg is on a different date (trip2_day > 0), collect
			// recheck data so we can still generate /searchpm URLs.
			// PHP's PackAndConnectionSearch fetches legs across dates; Go doesn't yet.
			if set.PackID > 0 && ok1 && !ok2 && set.Trip2Day > 0 {
				leg2Date := in.Filter.Date.AddDate(0, 0, set.Trip2Day).Format("2006-01-02")
				pp := domain.PendingPackRecheck{
					HeadTripKey: set.TripKey,
					ChunkKey:    buildRecheckChunkKey(leg1, 0), // use leg1's integration for chunk key
					Legs: []domain.PendingPackLeg{
						{TripKey: set.Trip1Key, Date: searchDate},
						{TripKey: set.Trip2Key, Date: leg2Date},
					},
				}
				if set.Trip3Key != nil && *set.Trip3Key != "" {
					leg3Date := searchDate
					if set.Trip3Day > 0 {
						leg3Date = in.Filter.Date.AddDate(0, 0, set.Trip3Day).Format("2006-01-02")
					}
					pp.Legs = append(pp.Legs, domain.PendingPackLeg{TripKey: *set.Trip3Key, Date: leg3Date})
				}
				pendingPacks = append(pendingPacks, pp)
			}
			continue
		}

		// PHP PackAndConnectionSearch::getSetRawTripsBySets() lines 657-684:
		// For packs (pack_id > 0), $connectionTrips = [null], so $connectionTrip = null.
		// This means godate, departureTime, and departure2Time are all null, skipping
		// the checks at lines 691, 697, and 715. Only non-pack connections use these checks.
		// PHP PackAndConnectionSearch lines 662-681: for non-pack connections,
		// the assembled trip's price comes from the HEAD composite trip (stored in
		// $rawTripConnectionBase), NOT from leg1's individual price. This is critical
		// because the head trip's price may be invalid (needs recheck) even when
		// leg1's individual price is valid.
		var headPrice *domain.TripPrice
		if set.PackID == 0 {
			// PHP iterates over ALL composite rows per set_id, checking godate AND
			// departure2_time together for EACH row. A connection is assembled only if
			// at least one composite row passes BOTH checks:
			//   line 691: $connectionTrip['godate'] === $rawTrip1['godate']
			//   line 715: $departure2Time !== null && $trip2['departure_time'] === $departure2Time
			// dep2=0 always rejects (departure_time > 0 !== 0).
			compositeRows := in.ConnectionCompositeRows[set.SetID]
			compositeMatch := false
			for _, row := range compositeRows {
				if row.Godate == leg1.Godate && row.Departure2Time > 0 && row.Departure2Time == leg2.DepartureTime {
					compositeMatch = true
					hp := row.HeadPrice
					headPrice = &hp
					break
				}
			}
			if !compositeMatch {
				continue
			}
		}

		// Validate connection: stop time between legs (min 30 minutes)
		leg1ArrTime := leg1.DepartureTime + leg1.Duration
		stopTime := leg2.DepartureTime - leg1ArrTime
		if stopTime < 30 {
			continue
		}

		// Validate max transit guarantee
		if set.Transit1Guarantee > 0 && stopTime > set.Transit1Guarantee {
			continue
		}

		// Assemble composite trip
		conn := leg1 // copy base from leg1
		setID := set.SetID
		conn.SetID = &setID
		conn.PackID = set.PackID
		conn.HeadTripKey = set.TripKey
		conn.ArrStationID = leg2.ArrStationID
		conn.ArrTimezoneName = leg2.ArrTimezoneName
		conn.ArrCountryID = leg2.ArrCountryID
		conn.ArrProvinceID = leg2.ArrProvinceID
		conn.Duration = leg1.Duration + stopTime + leg2.Duration
		conn.Departure2Time = leg2.DepartureTime
		conn.Arr = leg2.Arr
		conn.TripKey = set.TripKey

		// For non-pack connections, use the head trip's price instead of leg1's.
		// PHP PackAndConnectionSearch lines 662-681: $rawTripConnectionBase merges
		// the head trip's price over leg1's fields via PHP array + operator.
		if headPrice != nil {
			conn.Price = *headPrice
		}

		// Store per-leg station pairs for buy items and recheck grouping.
		conn.ConnectionLegs = []domain.LegPair{
			{TripKey: leg1.TripKey, FromID: leg1.DepStationID, ToID: leg1.ArrStationID, Godate: leg1.Godate},
			{TripKey: leg2.TripKey, FromID: leg2.DepStationID, ToID: leg2.ArrStationID, Godate: leg2.Godate},
		}

		// Handle 3-leg connections
		if set.Trip3Key != nil && *set.Trip3Key != "" {
			if leg3, ok3 := tripByKey[*set.Trip3Key]; ok3 {
				conn.ArrStationID = leg3.ArrStationID
				conn.ArrTimezoneName = leg3.ArrTimezoneName
				conn.ArrCountryID = leg3.ArrCountryID
				conn.ArrProvinceID = leg3.ArrProvinceID
				conn.Departure3Time = leg3.DepartureTime
				conn.Duration += leg3.Duration
				conn.Arr = leg3.Arr
				conn.ConnectionLegs = append(conn.ConnectionLegs, domain.LegPair{
					TripKey: leg3.TripKey, FromID: leg3.DepStationID, ToID: leg3.ArrStationID, Godate: leg3.Godate,
				})
			}
		}

		connections = append(connections, conn)
	}

	return connections, pendingPacks, nil
}

func (s *AssembleMultiLegStage) buildAutopacks(ctx context.Context, in AssembleMultiLegInput) ([]domain.RawTrip, error) {
	configs, err := s.autopackRepo.FindByPlaces(ctx, in.Filter.FromPlaceID, in.Filter.ToPlaceID)
	if err != nil {
		return nil, err
	}

	if len(configs) == 0 {
		return nil, nil
	}

	// Index direct trips by from/to station pairs
	type stationKey struct {
		from, to int
	}
	tripsByRoute := make(map[stationKey][]domain.RawTrip)
	for _, t := range in.DirectTrips {
		key := stationKey{t.DepStationID, t.ArrStationID}
		tripsByRoute[key] = append(tripsByRoute[key], t)
	}

	var autopacks []domain.RawTrip
	for _, cfg := range configs {
		for ri, route := range cfg.Routes {
			leg1Key := stationKey{route.Leg1.FromStationID, route.Leg1.ToStationID}
			leg2Key := stationKey{route.Leg2.FromStationID, route.Leg2.ToStationID}

			leg1Trips := tripsByRoute[leg1Key]
			leg2Trips := tripsByRoute[leg2Key]

			if len(leg1Trips) == 0 || len(leg2Trips) == 0 {
				continue
			}

			// Match legs by time tolerance
			for _, l1 := range leg1Trips {
				leg1ArrTime := l1.DepartureTime + l1.Duration

				for _, l2 := range leg2Trips {
					waitTime := l2.DepartureTime - leg1ArrTime
					if waitTime < 30 || waitTime > 480 {
						continue
					}

					// Assemble autopack trip
					ap := l1
					ap.ArrStationID = l2.ArrStationID
					ap.ArrTimezoneName = l2.ArrTimezoneName
					ap.ArrCountryID = l2.ArrCountryID
					ap.ArrProvinceID = l2.ArrProvinceID
					ap.Duration = l1.Duration + waitTime + l2.Duration
					ap.Departure2Time = l2.DepartureTime
					ap.Arr = l2.Arr
					ap.TripKey = fmt.Sprintf("P-%d-%d", cfg.AutopackID, ri)
					ap.ConnectionLegs = []domain.LegPair{
						{TripKey: l1.TripKey, FromID: l1.DepStationID, ToID: l1.ArrStationID, Godate: l1.Godate},
						{TripKey: l2.TripKey, FromID: l2.DepStationID, ToID: l2.ArrStationID, Godate: l2.Godate},
					}

					autopacks = append(autopacks, ap)
				}
			}
		}
	}

	return autopacks, nil
}
