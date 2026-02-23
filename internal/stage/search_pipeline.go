package stage

import (
	"context"
	"time"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/pipeline"
)

// SearchPipelineInput is the top-level input to the search pipeline.
type SearchPipelineInput struct {
	FromPlaceID  string
	ToPlaceID    string
	FromStations []int
	ToStations   []int
	Date         time.Time
	Params       domain.SearchParams
	Agent        domain.AgentContext
}

// SearchPipeline orchestrates all 9 stages.
type SearchPipeline struct {
	stage1 *ResolvePlacesStage
	stage2 *BuildFilterStage
	stage3 *QueryTripsStage
	stage4 *FilterRawTripsStage
	stage5a *AssembleMultiLegStage
	stage5b *EnrichRoundTripsStage
	stage6 *CollectRefDataStage
	stage7 *HydrateResultsStage
	stage8 *SortAndFinalizeStage
	stage9 *SerializeResponseStage
}

func NewSearchPipeline(
	stage1 *ResolvePlacesStage,
	stage2 *BuildFilterStage,
	stage3 *QueryTripsStage,
	stage4 *FilterRawTripsStage,
	stage5a *AssembleMultiLegStage,
	stage5b *EnrichRoundTripsStage,
	stage6 *CollectRefDataStage,
	stage7 *HydrateResultsStage,
	stage8 *SortAndFinalizeStage,
	stage9 *SerializeResponseStage,
) *SearchPipeline {
	return &SearchPipeline{
		stage1: stage1, stage2: stage2, stage3: stage3, stage4: stage4,
		stage5a: stage5a, stage5b: stage5b,
		stage6: stage6, stage7: stage7, stage8: stage8, stage9: stage9,
	}
}

// Execute runs the full search pipeline.
func (p *SearchPipeline) Execute(ctx context.Context, in SearchPipelineInput) (SearchResponse, error) {
	pc := pipeline.NewPipelineContext("")
	ctx = pipeline.WithPipelineContext(ctx, pc)

	// Stage 1: Resolve places
	resolved, err := pipeline.Run(ctx, p.stage1, ResolvePlacesInput{
		FromPlaceID:  in.FromPlaceID,
		ToPlaceID:    in.ToPlaceID,
		FromStations: in.FromStations,
		ToStations:   in.ToStations,
	})
	if err != nil {
		return SearchResponse{}, err
	}

	// Stage 2: Build filter
	filter, err := pipeline.Run(ctx, p.stage2, BuildFilterInput{
		ResolvedPlaces: resolved,
		SearchParams:   in.Params,
		Agent:          in.Agent,
		Date:           in.Date,
	})
	if err != nil {
		return SearchResponse{}, err
	}

	// Stage 3: Query trips
	rawResult, err := pipeline.Run(ctx, p.stage3, filter)
	if err != nil {
		return SearchResponse{}, err
	}
	// Stage 4: Filter raw trips
	filtered, err := pipeline.Run(ctx, p.stage4, rawResult)
	if err != nil {
		return SearchResponse{}, err
	}

	// Stage 5a+5b: Parallel multi-leg assembly and round trip enrichment
	multiLegInput := AssembleMultiLegInput{
		DirectTrips:   filtered.DirectTrips,
		ConnectionIDs: filtered.ConnectionIDs,
		Filter:        filtered.Filter,
	}
	roundTripInput := EnrichRoundTripsInput{
		DirectTrips: filtered.DirectTrips,
		Filter:      filtered.Filter,
	}

	merged, err := pipeline.RunParallelMerge(
		ctx,
		multiLegInput,
		// Adapter: 5a takes AssembleMultiLegInput
		p.stage5a,
		// Adapter: 5b takes same input shape, convert
		&enrichRoundTripsAdapter{stage: p.stage5b, input: roundTripInput},
		// Merge function
		func(ml MultiLegTrips, rt RoundTripEnrichedTrips) CollectRefDataInput {
			all := make([]domain.RawTrip, 0, len(rt.Trips)+len(ml.Connections)+len(ml.Autopacks))
			all = append(all, rt.Trips...)
			all = append(all, ml.Connections...)
			all = append(all, ml.Autopacks...)
			return CollectRefDataInput{
				AllTrips: all,
				Filter:   filtered.Filter,
			}
		},
	)
	if err != nil {
		return SearchResponse{}, err
	}

	// Stage 6: Collect reference data
	enriched, err := pipeline.Run(ctx, p.stage6, merged)
	if err != nil {
		return SearchResponse{}, err
	}

	// Stage 7: Hydrate results
	hydrated, err := pipeline.Run(ctx, p.stage7, enriched)
	if err != nil {
		return SearchResponse{}, err
	}

	// Stage 8: Sort and finalize
	final, err := pipeline.Run(ctx, p.stage8, hydrated)
	if err != nil {
		return SearchResponse{}, err
	}

	// Stage 9: Serialize response
	return pipeline.Run(ctx, p.stage9, final)
}

// enrichRoundTripsAdapter adapts EnrichRoundTripsStage to accept AssembleMultiLegInput.
type enrichRoundTripsAdapter struct {
	stage *EnrichRoundTripsStage
	input EnrichRoundTripsInput
}

func (a *enrichRoundTripsAdapter) Name() string { return a.stage.Name() }

func (a *enrichRoundTripsAdapter) Execute(ctx context.Context, _ AssembleMultiLegInput) (RoundTripEnrichedTrips, error) {
	return a.stage.Execute(ctx, a.input)
}
