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
	DirectTrips   []domain.RawTrip
	ConnectionIDs []int
	Filter        domain.SearchFilter
}

// MultiLegTrips is the output of Stage 5a.
type MultiLegTrips struct {
	Connections []domain.RawTrip
	Autopacks   []domain.RawTrip
}

// AssembleMultiLegStage builds connections from trip_pool4_set and autopacks.
type AssembleMultiLegStage struct {
	tripPoolSetRepo *repository.TripPoolSetRepo
	autopackRepo    *repository.AutopackRepo
	regionResolver  db.RegionResolver
}

func NewAssembleMultiLegStage(
	tripPoolSetRepo *repository.TripPoolSetRepo,
	autopackRepo *repository.AutopackRepo,
	regionResolver db.RegionResolver,
) *AssembleMultiLegStage {
	return &AssembleMultiLegStage{
		tripPoolSetRepo: tripPoolSetRepo,
		autopackRepo:    autopackRepo,
		regionResolver:  regionResolver,
	}
}

func (s *AssembleMultiLegStage) Name() string { return "assemble_multi_leg" }

func (s *AssembleMultiLegStage) Execute(ctx context.Context, in AssembleMultiLegInput) (MultiLegTrips, error) {
	var out MultiLegTrips

	// Build connections from trip_pool4_set
	if len(in.ConnectionIDs) > 0 {
		conns, err := s.buildConnections(ctx, in)
		if err != nil {
			return out, err
		}
		out.Connections = conns
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

func (s *AssembleMultiLegStage) buildConnections(ctx context.Context, in AssembleMultiLegInput) ([]domain.RawTrip, error) {
	// Determine region from first departure station
	region := db.DefaultRegion
	if len(in.Filter.FromStationIDs) > 0 {
		region = s.regionResolver.ResolveByStationID(in.Filter.FromStationIDs[0])
	}

	sets, err := s.tripPoolSetRepo.FindBySetIDs(ctx, region, in.ConnectionIDs)
	if err != nil {
		return nil, err
	}

	// Build a trip lookup by trip key for matching legs
	tripByKey := make(map[string]domain.RawTrip, len(in.DirectTrips))
	for _, t := range in.DirectTrips {
		tripByKey[t.TripKey] = t
	}

	var connections []domain.RawTrip
	for _, set := range sets {
		// Find leg trips in the direct trips
		leg1, ok1 := tripByKey[set.Trip1Key]
		leg2, ok2 := tripByKey[set.Trip2Key]
		if !ok1 || !ok2 {
			continue
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
		conn.ArrStationID = leg2.ArrStationID
		conn.ArrTimezoneName = leg2.ArrTimezoneName
		conn.ArrCountryID = leg2.ArrCountryID
		conn.ArrProvinceID = leg2.ArrProvinceID
		conn.Duration = leg1.Duration + stopTime + leg2.Duration
		conn.Departure2Time = leg2.DepartureTime
		conn.Arr = leg2.Arr
		conn.TripKey = set.TripKey

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
			}
		}

		connections = append(connections, conn)
	}

	return connections, nil
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

					autopacks = append(autopacks, ap)
				}
			}
		}
	}

	return autopacks, nil
}
