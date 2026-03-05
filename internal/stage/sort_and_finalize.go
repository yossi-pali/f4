package stage

import (
	"context"
	"sort"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/pipeline"
)

// FinalResults is the output of Stage 8b (sort). Identical to MergedResults.
type FinalResults struct {
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

// SortAndFinalizeStage sorts merged results: bookable → valid price → special deal → rank score.
type SortAndFinalizeStage struct{}

func NewSortAndFinalizeStage() *SortAndFinalizeStage {
	return &SortAndFinalizeStage{}
}

func (s *SortAndFinalizeStage) Name() string { return "sort_and_finalize" }

func (s *SortAndFinalizeStage) Execute(ctx context.Context, in MergedResults) (FinalResults, error) {
	pc := pipeline.FromContext(ctx)
	t := pc.StartTimer("sort_and_finalize", "sort")

	// Sort: bookable first → valid price → special deals → rank score
	sort.SliceStable(in.Trips, func(i, j int) bool {
		a, b := in.Trips[i], in.Trips[j]
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
	t.Stop()

	return FinalResults{
		Trips:               in.Trips,
		RecheckTripKeys:     in.RecheckTripKeys,
		RecheckGroups:       in.RecheckGroups,
		PackRecheckGroups:   in.PackRecheckGroups,
		PresentIntegrations: in.PresentIntegrations,
		Operators:           in.Operators,
		Stations:            in.Stations,
		Classes:             in.Classes,
		ToProvinceName:      in.ToProvinceName,
	}, nil
}
