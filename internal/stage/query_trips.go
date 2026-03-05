package stage

import (
	"context"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/pipeline"
	"github.com/12go/f4/internal/repository"
)

// RawTripsResult is the output of Stage 3.
type RawTripsResult struct {
	Trips       []domain.RawTrip
	Region      string
	QueryTimeMs int64
}

// QueryTripsStage executes the main SQL query against regional DB.
type QueryTripsStage struct {
	tripPoolRepo *repository.TripPoolRepo
}

func NewQueryTripsStage(tripPoolRepo *repository.TripPoolRepo) *QueryTripsStage {
	return &QueryTripsStage{tripPoolRepo: tripPoolRepo}
}

func (s *QueryTripsStage) Name() string { return "query_trips" }

func (s *QueryTripsStage) Execute(ctx context.Context, filter domain.SearchFilter) (RawTripsResult, error) {
	var result RawTripsResult

	if filter.IsNotPossible || len(filter.FromStationIDs) == 0 || len(filter.ToStationIDs) == 0 {
		return result, nil
	}

	pc := pipeline.FromContext(ctx)
	const stage = "query_trips"

	t := pc.StartTimer(stage, "sql_execute")

	params := repository.SearchParams{
		FromStationIDs:     filter.FromStationIDs,
		ToStationIDs:       filter.ToStationIDs,
		FromPlaceID:        filter.FromPlaceID,
		ToPlaceID:          filter.ToPlaceID,
		GodateString:       filter.Date.Format("2006-01-02"),
		SeatsAdult:         filter.SeatsAdult,
		SeatsChild:         filter.SeatsChild,
		SeatsInfant:        filter.SeatsInfant,
		AgentID:            filter.AgentID,
		Lang:               filter.Lang,
		FXCode:             filter.FXCode,
		RecheckLevel:       filter.RecheckLevel,
		PriceMode:          filter.PriceMode,
		OperatorIDs:        filter.OperatorIDs,
		SellerIDs:          filter.SellerIDs,
		VehclassIDs:        filter.VehclassIDs,
		ClassIDs:           filter.ClassIDs,
		CountryIDs:         filter.CountryIDs,
		ExcludeOperatorIDs: filter.ExcludeOperatorIDs,
		ExcludeSellerIDs:   filter.ExcludeSellerIDs,
		ExcludeVehclassIDs: filter.ExcludeVehclassIDs,
		ExcludeClassIDs:    filter.ExcludeClassIDs,
		ExcludeCountryIDs:  filter.ExcludeCountryIDs,
		IntegrationCode:    filter.IntegrationCode,
		TripKeys:           filter.TripKeys,
		OnlyDirect:         filter.OnlyDirect,
	}

	trips, err := s.tripPoolRepo.Search(ctx, params)
	sqlDur := t.Stop()
	if err != nil {
		return result, err
	}

	result.Trips = trips
	result.QueryTimeMs = sqlDur.Milliseconds()

	return result, nil
}
