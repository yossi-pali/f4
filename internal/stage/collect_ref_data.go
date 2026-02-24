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
	Images    *domain.ImageCollection
	Filter    domain.SearchFilter
}

// CollectRefDataStage batch-loads all reference data (operators, stations, classes, images) in parallel.
type CollectRefDataStage struct {
	stationRepo  *repository.StationRepo
	operatorRepo *repository.OperatorRepo
	classRepo    *repository.ClassRepo
	imageRepo    *repository.ImageRepo
}

func NewCollectRefDataStage(
	stationRepo *repository.StationRepo,
	operatorRepo *repository.OperatorRepo,
	classRepo *repository.ClassRepo,
	imageRepo *repository.ImageRepo,
) *CollectRefDataStage {
	return &CollectRefDataStage{
		stationRepo:  stationRepo,
		operatorRepo: operatorRepo,
		classRepo:    classRepo,
		imageRepo:    imageRepo,
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
	routeIDSet := make(map[int]struct{})
	pairSet := make(map[repository.OperatorClassPair]struct{})

	for _, t := range in.AllTrips {
		operatorIDSet[t.OperatorID] = struct{}{}
		stationIDSet[t.DepStationID] = struct{}{}
		stationIDSet[t.ArrStationID] = struct{}{}
		classIDSet[t.ClassID] = struct{}{}
		if t.RouteID > 0 {
			routeIDSet[t.RouteID] = struct{}{}
		}
		pairSet[repository.OperatorClassPair{OperatorID: t.OperatorID, ClassID: t.ClassID}] = struct{}{}
	}

	operatorIDs := setToSlice(operatorIDSet)
	stationIDs := setToSlice(stationIDSet)
	classIDs := setToSlice(classIDSet)
	routeIDs := setToSlice(routeIDSet)
	pairs := make([]repository.OperatorClassPair, 0, len(pairSet))
	for p := range pairSet {
		pairs = append(pairs, p)
	}

	// Load all reference data in parallel
	g, ctx := errgroup.WithContext(ctx)

	var logos map[int][]any
	var ratings map[int]repository.OperatorRating

	g.Go(func() error {
		var err error
		out.Operators, err = s.operatorRepo.FindByIDs(ctx, operatorIDs)
		return err
	})

	g.Go(func() error {
		var err error
		ratings, err = s.operatorRepo.FindOperatorRatings(ctx, operatorIDs)
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

	// Image loading
	g.Go(func() error {
		var err error
		logos, err = s.imageRepo.FindOperatorLogos(ctx, operatorIDs)
		return err
	})

	g.Go(func() error {
		var err error
		out.Images, err = s.imageRepo.FindClassImages(ctx, pairs)
		if err != nil {
			return err
		}
		// Load custom class images into same collection
		if err := s.imageRepo.LoadCustomClassImages(ctx, out.Images, pairs); err != nil {
			return err
		}
		// Load route images into same collection
		return s.imageRepo.LoadRouteImages(ctx, out.Images, routeIDs)
	})

	if err := g.Wait(); err != nil {
		return out, err
	}

	// Merge logos and ratings into operators
	for opID, logo := range logos {
		if op, ok := out.Operators[opID]; ok {
			op.Logo = logo
			out.Operators[opID] = op
		}
	}
	for opID, r := range ratings {
		if op, ok := out.Operators[opID]; ok {
			op.RatingAvg = r.Rating
			op.RatingCount = r.RatingsCount
			out.Operators[opID] = op
		}
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
