package stage

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/repository"
)

// CollectRefDataInput is the input for Stage 6.
type CollectRefDataInput struct {
	AllTrips []domain.RawTrip
	Filter   domain.SearchFilter
}

// EnrichedTrips is the output of Stage 6.
type EnrichedTrips struct {
	Trips     []domain.RawTrip
	Operators map[int]domain.Operator
	Stations  map[int]domain.Station
	Classes   map[int]domain.VehicleClass
	Filter    domain.SearchFilter
}

// CollectRefDataStage batch-loads all reference data (operators, stations, classes) in parallel.
type CollectRefDataStage struct {
	stationRepo  *repository.StationRepo
	operatorRepo *repository.OperatorRepo
	classRepo    *repository.ClassRepo
}

func NewCollectRefDataStage(
	stationRepo *repository.StationRepo,
	operatorRepo *repository.OperatorRepo,
	classRepo *repository.ClassRepo,
) *CollectRefDataStage {
	return &CollectRefDataStage{
		stationRepo:  stationRepo,
		operatorRepo: operatorRepo,
		classRepo:    classRepo,
	}
}

func (s *CollectRefDataStage) Name() string { return "collect_ref_data" }

func (s *CollectRefDataStage) Execute(ctx context.Context, in CollectRefDataInput) (EnrichedTrips, error) {
	out := EnrichedTrips{
		Trips:  in.AllTrips,
		Filter: in.Filter,
	}

	if len(in.AllTrips) == 0 {
		out.Operators = map[int]domain.Operator{}
		out.Stations = map[int]domain.Station{}
		out.Classes = map[int]domain.VehicleClass{}
		return out, nil
	}

	// Collect unique IDs
	operatorIDSet := make(map[int]struct{})
	stationIDSet := make(map[int]struct{})
	classIDSet := make(map[int]struct{})

	for _, t := range in.AllTrips {
		operatorIDSet[t.OperatorID] = struct{}{}
		stationIDSet[t.DepStationID] = struct{}{}
		stationIDSet[t.ArrStationID] = struct{}{}
		classIDSet[t.ClassID] = struct{}{}
	}

	operatorIDs := setToSlice(operatorIDSet)
	stationIDs := setToSlice(stationIDSet)
	classIDs := setToSlice(classIDSet)

	// Load all reference data in parallel
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		out.Operators, err = s.operatorRepo.FindByIDs(ctx, operatorIDs)
		return err
	})

	g.Go(func() error {
		var err error
		out.Stations, err = s.stationRepo.FindStationsByIDs(ctx, stationIDs)
		return err
	})

	g.Go(func() error {
		var err error
		out.Classes, err = s.classRepo.FindByIDs(ctx, classIDs)
		return err
	})

	if err := g.Wait(); err != nil {
		return out, err
	}

	return out, nil
}

func setToSlice(m map[int]struct{}) []int {
	s := make([]int, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	return s
}
