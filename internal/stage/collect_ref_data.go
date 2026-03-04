package stage

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/repository"
)

// CollectRefDataInput is the input for Stage 6.
type CollectRefDataInput struct {
	AllTrips                []domain.RawTrip
	AllStationIDs           []int // station IDs from ALL raw trips (before filtering), for station collection
	PreFilterRecheckEntries []domain.PreFilterRecheckEntry // recheck data from ALL raw trips (before filtering)
	PendingPackRechecks     []domain.PendingPackRecheck    // multi-day packs that couldn't be assembled
	Filter                  domain.SearchFilter
}

// EnrichedTrips is the output of Stage 6.
type EnrichedTrips struct {
	Trips                   []domain.RawTrip
	Operators               map[int]domain.Operator
	Stations                map[int]domain.Station
	Classes                 map[int]domain.VehicleClass
	Images                  *domain.ImageCollection
	ReasonTexts             map[int]string // reason_id → translated text (e.g., "This trip is not bookable")
	ManualIntegrationID     int // integration_id for integration_code='manual' (fallback for sellers without integration row)
	PreFilterRecheckEntries []domain.PreFilterRecheckEntry // passed through from FilterRawTrips
	PendingPackRechecks     []domain.PendingPackRecheck    // multi-day packs that couldn't be assembled
	Filter                  domain.SearchFilter
}

// CollectRefDataStage batch-loads all reference data (operators, stations, classes, images) in parallel.
type CollectRefDataStage struct {
	stationRepo     *repository.StationRepo
	operatorRepo    *repository.OperatorRepo
	classRepo       *repository.ClassRepo
	imageRepo       *repository.ImageRepo
	reasonRepo      *repository.ReasonRepo
	integrationRepo *repository.IntegrationRepo
	tranRepo        *repository.TranRepo
}

func NewCollectRefDataStage(
	stationRepo *repository.StationRepo,
	operatorRepo *repository.OperatorRepo,
	classRepo *repository.ClassRepo,
	imageRepo *repository.ImageRepo,
	reasonRepo *repository.ReasonRepo,
	integrationRepo *repository.IntegrationRepo,
	tranRepo *repository.TranRepo,
) *CollectRefDataStage {
	return &CollectRefDataStage{
		stationRepo:     stationRepo,
		operatorRepo:    operatorRepo,
		classRepo:       classRepo,
		imageRepo:       imageRepo,
		reasonRepo:      reasonRepo,
		integrationRepo: integrationRepo,
		tranRepo:        tranRepo,
	}
}

func (s *CollectRefDataStage) Name() string { return "collect_ref_data" }

func (s *CollectRefDataStage) Execute(ctx context.Context, in CollectRefDataInput) (EnrichedTrips, error) {
	out := EnrichedTrips{
		Trips:                   in.AllTrips,
		PreFilterRecheckEntries: in.PreFilterRecheckEntries,
		PendingPackRechecks:     in.PendingPackRechecks,
		Filter:                  in.Filter,
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

	reasonIDSet := make(map[int]struct{})

	for _, t := range in.AllTrips {
		// PHP's prepareRawTrips collects operators/classes only from trips
		// that survive its filtering. Assembled connections (set_id > 0) are
		// built from leg trip data and may include operators/classes that PHP
		// filters out during connection assembly (e.g., godate mismatches,
		// departure time checks). Only collect operators/classes from direct
		// trips to match PHP behavior.
		isConnection := t.SetID != nil && *t.SetID > 0
		if !isConnection {
			operatorIDSet[t.OperatorID] = struct{}{}
			classIDSet[t.ClassID] = struct{}{}
			pairSet[repository.OperatorClassPair{OperatorID: t.OperatorID, ClassID: t.ClassID}] = struct{}{}
		}
		stationIDSet[t.DepStationID] = struct{}{}
		stationIDSet[t.ArrStationID] = struct{}{}
		if t.RouteID > 0 {
			routeIDSet[t.RouteID] = struct{}{}
		}
		if t.Price.ReasonID > 0 {
			reasonIDSet[t.Price.ReasonID] = struct{}{}
		}
	}

	// Also include station IDs collected from all raw trips before filtering.
	// PHP collects stations before filtering meta/daytrip trips, so some stations
	// appear in the response even when no visible trip references them.
	for _, id := range in.AllStationIDs {
		stationIDSet[id] = struct{}{}
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
	origCtx := ctx
	g, ctx := errgroup.WithContext(ctx)

	var logos map[int][]any
	var ratings map[int]repository.OperatorRating
	var classTranslations map[string]string
	var weightOverrides map[int]int

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

	g.Go(func() error {
		reasonIDs := setToSlice(reasonIDSet)
		var err error
		out.ReasonTexts, err = s.reasonRepo.FindReasonTexts(ctx, reasonIDs, in.Filter.Lang)
		return err
	})

	// Manual integration lookup (for chunk_key computation)
	g.Go(func() error {
		manual, err := s.integrationRepo.FindByCode(ctx, "manual")
		if err == nil {
			out.ManualIntegrationID = manual.IntegrationID
		}
		return nil // non-fatal: fallback to 0
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

	// Station weight overrides from page_override table
	g.Go(func() error {
		var err error
		weightOverrides, err = s.stationRepo.FindStationWeightOverrides(ctx, stationIDs, in.Filter.PageURL)
		if err != nil {
			weightOverrides = nil // non-fatal: use original weights
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return out, err
	}

	// Translate class names (matching PHP ClassCollector: $this->tran->translate($class['class_name']))
	// Done after g.Wait because we need loaded class names first.
	classNames := make([]string, 0, len(out.Classes))
	for _, c := range out.Classes {
		classNames = append(classNames, c.Name)
	}
	classTranslations, _ = s.tranRepo.TranslateMany(origCtx, classNames, in.Filter.Lang)
	if len(classTranslations) > 0 {
		for id, c := range out.Classes {
			if translated, ok := classTranslations[c.Name]; ok {
				c.Name = translated
				out.Classes[id] = c
			}
		}
	}

	// Apply station weight overrides
	if len(weightOverrides) > 0 {
		for id, st := range out.Stations {
			if w, ok := weightOverrides[id]; ok {
				st.Weight = w
				out.Stations[id] = st
			}
		}
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
