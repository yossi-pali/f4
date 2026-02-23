package stage

import (
	"context"
	"time"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/feature"
	"github.com/12go/f4/internal/repository"
)

// BuildFilterInput is the input for Stage 2.
type BuildFilterInput struct {
	ResolvedPlaces ResolvedPlaces
	SearchParams   domain.SearchParams
	Agent          domain.AgentContext
	Date           time.Time
}

// dataSecProvider is a local interface for security restriction queries.
type dataSecProvider interface {
	GetRestrictions(ctx context.Context, agentID int) (repository.SecurityRestrictions, error)
}

// whiteLabelProvider is a local interface for white-label config queries.
type whiteLabelProvider interface {
	GetConfig(ctx context.Context, agentID int) (repository.WhiteLabelConfig, error)
}

// BuildFilterStage builds the complete search filter from resolved places and params.
type BuildFilterStage struct {
	dataSecRepo    dataSecProvider
	whiteLabelRepo whiteLabelProvider
	features       *feature.Flags
}

func NewBuildFilterStage(
	dataSecRepo dataSecProvider,
	whiteLabelRepo whiteLabelProvider,
	features *feature.Flags,
) *BuildFilterStage {
	return &BuildFilterStage{
		dataSecRepo:    dataSecRepo,
		whiteLabelRepo: whiteLabelRepo,
		features:       features,
	}
}

func (s *BuildFilterStage) Name() string { return "build_filter" }

func (s *BuildFilterStage) Execute(ctx context.Context, in BuildFilterInput) (domain.SearchFilter, error) {
	rp := in.ResolvedPlaces
	p := in.SearchParams

	filter := domain.SearchFilter{
		FromPlaceID:    rp.FromPlaceID,
		ToPlaceID:      rp.ToPlaceID,
		FromStationIDs: rp.FromStationIDs,
		ToStationIDs:   rp.ToStationIDs,
		Date:           in.Date,
		IsNotPossible:  rp.IsNotPossible,

		// Defaults
		SeatsAdult:  max(p.SeatsAdult, 1),
		SeatsChild:  p.SeatsChild,
		SeatsInfant: p.SeatsInfant,
		FXCode:      p.FXCode,
		Lang:        p.Lang,
		AgentID:     in.Agent.AgentID,

		OnlyDirect:      p.Direct,
		OnlyPairs:       p.OnlyPairs,
		WithNonBookable: p.WithNonBookable,
		IntegrationCode: p.IntegrationCode,
		RecheckAmount:   p.RecheckAmount,
		WithAdminLinks:  in.Agent.IsAdmin,
		WithAutopacks:   s.features.Enabled(feature.Autopacks),
	}

	if filter.Lang == "" {
		filter.Lang = "en"
	}

	// Determine recheck level
	if in.Agent.IsBot {
		filter.RecheckLevel = domain.RecheckNever
	} else {
		filter.RecheckLevel = domain.RecheckBySettings
	}

	// Apply security restrictions
	sec, err := s.dataSecRepo.GetRestrictions(ctx, in.Agent.AgentID)
	if err != nil {
		return filter, err
	}
	filter.OperatorIDs = sec.OperatorIDs
	filter.SellerIDs = sec.SellerIDs
	filter.CountryIDs = sec.CountryIDs
	filter.VehclassIDs = sec.VehclassIDs
	filter.ClassIDs = sec.ClassIDs
	filter.ExcludeOperatorIDs = sec.ExcludeOperatorIDs
	filter.ExcludeSellerIDs = sec.ExcludeSellerIDs
	filter.ExcludeCountryIDs = sec.ExcludeCountryIDs
	filter.ExcludeVehclassIDs = sec.ExcludeVehclassIDs
	filter.ExcludeClassIDs = sec.ExcludeClassIDs

	// Apply white-label restrictions
	wlCfg, err := s.whiteLabelRepo.GetConfig(ctx, in.Agent.AgentID)
	if err != nil {
		return filter, err
	}
	if len(wlCfg.OperatorIDs) > 0 {
		filter.OperatorIDs = intersectInts(filter.OperatorIDs, wlCfg.OperatorIDs)
		if len(filter.OperatorIDs) == 0 && len(wlCfg.OperatorIDs) > 0 {
			filter.IsNotPossible = true
		}
	}

	// Vehclass filter from query param
	if p.VehclassID != "" {
		filter.VehclassIDs = append(filter.VehclassIDs, p.VehclassID)
	}

	return filter, nil
}

// intersectInts returns the intersection of two int slices. If a is empty, returns b.
func intersectInts(a, b []int) []int {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	set := make(map[int]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	var result []int
	for _, v := range b {
		if _, ok := set[v]; ok {
			result = append(result, v)
		}
	}
	return result
}
