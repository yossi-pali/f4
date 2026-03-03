package stage

import (
	"context"

	"github.com/12go/f4/internal/domain"
)

// FilteredTrips is the output of Stage 4.
type FilteredTrips struct {
	DirectTrips      []domain.RawTrip
	ConnectionIDs    []int // set_id values for Stage 5a
	AllStationIDs    []int // station IDs from ALL raw trips (before filtering), for station collection
	Filter           domain.SearchFilter
}

// FilterRawTripsStage removes hidden, meta, daytrip duplicates, and separates connections.
type FilterRawTripsStage struct{}

func NewFilterRawTripsStage() *FilterRawTripsStage { return &FilterRawTripsStage{} }

func (s *FilterRawTripsStage) Name() string { return "filter_raw_trips" }

func (s *FilterRawTripsStage) Execute(_ context.Context, in RawTripsResult) (FilteredTrips, error) {
	out := FilteredTrips{Filter: in.Filter}

	if len(in.Trips) == 0 {
		return out, nil
	}

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

	// Track operator+station pairs for daytrip detection
	type stationPair struct {
		operatorID int
		fromID     int
		toID       int
	}
	seen := make(map[stationPair]bool, len(in.Trips))

	connectionIDSet := make(map[int]bool)
	direct := make([]domain.RawTrip, 0, len(in.Trips))

	for i := range in.Trips {
		trip := &in.Trips[i]

		// Filter out hidden departures
		if trip.DepHideDeparture {
			continue
		}

		// Filter out meta operators
		if trip.IsMeta {
			continue
		}

		// Separate connections (set_id != nil)
		if trip.SetID != nil && *trip.SetID > 0 {
			connectionIDSet[*trip.SetID] = true
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

	out.DirectTrips = direct
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
