package stage

import (
	"context"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/pipeline"
)

// CompositeRow stores godate, departure2_time, and the price from a single composite
// price row. PHP iterates over ALL composite rows per set_id, checking godate AND dep2
// together. PHP also uses the head trip's price for the assembled connection (see
// PackAndConnectionSearch lines 662-681: $rawTripConnectionBase includes the head's
// price, which overrides leg1's price via PHP array + operator).
type CompositeRow struct {
	Godate         int64
	Departure2Time int
	HeadPrice      domain.TripPrice // the head composite trip's price (from trip_pool4_price)
}

// FilteredTrips is the output of Stage 4.
type FilteredTrips struct {
	DirectTrips              []domain.RawTrip
	ConnectionIDs            []int                      // set_id values for Stage 5a
	ConnectionCompositeRows  map[int][]CompositeRow     // set_id → composite rows with (godate, dep2) tuples
	AllStationIDs            []int                      // station IDs from ALL raw trips (before filtering), for station collection
	PreFilterRecheckEntries  []domain.PreFilterRecheckEntry // recheck data from ALL raw trips (before filtering)
	Filter                   domain.SearchFilter
}

// FilterRawTripsStage removes hidden, meta, daytrip duplicates, and separates connections.
type FilterRawTripsStage struct{}

func NewFilterRawTripsStage() *FilterRawTripsStage { return &FilterRawTripsStage{} }

func (s *FilterRawTripsStage) Name() string { return "filter_raw_trips" }

func (s *FilterRawTripsStage) Execute(ctx context.Context, in RawTripsResult) (FilteredTrips, error) {
	out := FilteredTrips{Filter: in.Filter}

	if len(in.Trips) == 0 {
		return out, nil
	}

	pc := pipeline.FromContext(ctx)
	const stage = "filter_raw_trips"
	t := pc.StartTimer(stage, "station_collect")

	// Collect station IDs from ALL raw trips before filtering.
	// PHP collects stations in prepareRawTrips which runs before trip-level
	// filters like meta/daytrip, so some stations appear in the response
	// even if no visible trip references them.
	allStationSet := make(map[int]struct{}, len(in.Trips)*2)
	for _, t := range in.Trips {
		// PHP Search.php lines 291-308: only dep_hide trips are excluded before station collection
		if t.DepHideDeparture {
			continue
		}
		allStationSet[t.DepStationID] = struct{}{}
		allStationSet[t.ArrStationID] = struct{}{}
	}
	out.AllStationIDs = make([]int, 0, len(allStationSet))
	for id := range allStationSet {
		out.AllStationIDs = append(out.AllStationIDs, id)
	}

	// PHP originalCollection pattern: collect recheck data from direct raw trips
	// before filtering. PHP Search.php lines 297-303: connection entries (set_id != 0)
	// are unset() from $rawTrips BEFORE ChiefCook processes them, so they NEVER
	// reach the recheck originalCollection. Only direct trips (set_id == 0/null),
	// including those later filtered out (meta, daytrip duplicates, etc.), contribute.
	for _, t := range in.Trips {
		if t.DepHideDeparture {
			continue // PHP also skips these
		}
		// PHP excludes connection entries from recheck entirely.
		if t.SetID != nil && *t.SetID > 0 {
			continue
		}
		if !t.Price.IsValid {
			out.PreFilterRecheckEntries = append(out.PreFilterRecheckEntries, domain.PreFilterRecheckEntry{
				DepStationID:    t.DepStationID,
				ArrStationID:    t.ArrStationID,
				IntegrationCode: t.IntegrationCode,
				IntegrationID:   t.IntegrationID,
				ChunkKeyRaw:     t.ChunkKey,
				VehclassID:      t.VehclassID,
				Godate:          t.Godate,
				OperatorID:      t.OperatorID,
				ClassID:         t.ClassID,
				OfficialID:      t.OfficialID,
			})
		}
	}

	t.Stop()
	t = pc.StartTimer(stage, "filter_loop")

	// Track operator+station pairs for daytrip detection
	type stationPair struct {
		operatorID int
		fromID     int
		toID       int
	}
	seen := make(map[stationPair]bool, len(in.Trips))

	connectionIDSet := make(map[int]bool)
	connectionCompositeRows := make(map[int][]CompositeRow)
	direct := make([]domain.RawTrip, 0, len(in.Trips))

	for i := range in.Trips {
		trip := &in.Trips[i]

		// Filter out hidden departures
		if trip.DepHideDeparture {
			continue
		}

		// Separate connections (set_id != nil) BEFORE meta check.
		// PHP Search.php line 297-303: composites are unset() from rawTrips
		// and their set_ids collected BEFORE any meta/operator filtering.
		// Meta filtering happens later (ChiefCook), so meta composites
		// must still be separated as connections here.
		if trip.SetID != nil && *trip.SetID > 0 {
			setID := *trip.SetID
			connectionIDSet[setID] = true
			// PHP PackAndConnectionSearch::getSetRawTripsBySets() iterates over ALL
			// composite rows per set_id, checking godate AND departure2_time together
			// for each row. Store all (godate, dep2) tuples to enable correlated checks.
			connectionCompositeRows[setID] = append(connectionCompositeRows[setID], CompositeRow{
				Godate:         trip.Godate,
				Departure2Time: trip.Departure2Time,
				HeadPrice:      trip.Price,
			})
			continue
		}

		// Filter out meta operators (only for direct trips, after composite separation)
		if trip.IsMeta {
			continue
		}

		// Only pairs filter: keep only trips matching requested station pairs
		if in.Filter.OnlyPairs {
			if !isInStationPairs(trip.DepStationID, trip.ArrStationID, in.Filter.FromStationIDs, in.Filter.ToStationIDs) {
				continue
			}
		}

		// Daytrip detection: remove reversed trips from same operator on same day
		pair := stationPair{trip.OperatorID, trip.DepStationID, trip.ArrStationID}
		reversePair := stationPair{trip.OperatorID, trip.ArrStationID, trip.DepStationID}

		if seen[reversePair] {
			continue
		}
		seen[pair] = true

		direct = append(direct, *trip)
	}

	t.Stop()

	out.DirectTrips = direct
	out.ConnectionCompositeRows = connectionCompositeRows
	for id := range connectionIDSet {
		out.ConnectionIDs = append(out.ConnectionIDs, id)
	}

	return out, nil
}

func isInStationPairs(fromID, toID int, fromIDs, toIDs []int) bool {
	fromMatch := false
	for _, id := range fromIDs {
		if id == fromID {
			fromMatch = true
			break
		}
	}
	if !fromMatch {
		return false
	}
	for _, id := range toIDs {
		if id == toID {
			return true
		}
	}
	return false
}
