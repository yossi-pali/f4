package stage

import (
	"context"
	"testing"
	"time"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/feature"
	"github.com/12go/f4/internal/repository"
)

// mockDataSecRepo returns configurable SecurityRestrictions.
type mockDataSecRepo struct {
	restrictions repository.SecurityRestrictions
	err          error
}

func (m *mockDataSecRepo) GetRestrictions(_ context.Context, _ int) (repository.SecurityRestrictions, error) {
	return m.restrictions, m.err
}

// mockWhiteLabelRepo returns configurable WhiteLabelConfig.
type mockWhiteLabelRepo struct {
	config repository.WhiteLabelConfig
	err    error
}

func (m *mockWhiteLabelRepo) GetConfig(_ context.Context, _ int) (repository.WhiteLabelConfig, error) {
	return m.config, m.err
}

func TestBuildFilterStage_BasicFilter(t *testing.T) {
	dataSec := &mockDataSecRepo{}
	whiteLabel := &mockWhiteLabelRepo{}
	flags := feature.New(map[string]bool{feature.Autopacks: true})

	stage := &BuildFilterStage{
		dataSecRepo:    dataSec,
		whiteLabelRepo: whiteLabel,
		features:       flags,
	}

	date, _ := time.Parse("2006-01-02", "2023-09-09")
	in := BuildFilterInput{
		ResolvedPlaces: ResolvedPlaces{
			FromPlaceID:    "100",
			ToPlaceID:      "200p",
			FromStationIDs: []int{100},
			ToStationIDs:   []int{201, 202, 203},
		},
		SearchParams: domain.SearchParams{
			Lang:       "",
			SeatsAdult: 1,
		},
		Agent: domain.AgentContext{},
		Date:  date,
	}

	filter, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if filter.FromPlaceID != "100" {
		t.Errorf("FromPlaceID = %q, want %q", filter.FromPlaceID, "100")
	}
	if filter.ToPlaceID != "200p" {
		t.Errorf("ToPlaceID = %q, want %q", filter.ToPlaceID, "200p")
	}
	if len(filter.FromStationIDs) != 1 || filter.FromStationIDs[0] != 100 {
		t.Errorf("FromStationIDs = %v, want [100]", filter.FromStationIDs)
	}
	if len(filter.ToStationIDs) != 3 {
		t.Errorf("ToStationIDs len = %d, want 3", len(filter.ToStationIDs))
	}
	if filter.SeatsAdult != 1 {
		t.Errorf("SeatsAdult = %d, want 1", filter.SeatsAdult)
	}
	if filter.Lang != "en" {
		t.Errorf("Lang = %q, want %q", filter.Lang, "en")
	}
	if filter.OnlyPairs {
		t.Error("expected OnlyPairs=false")
	}
	if filter.WithAdminLinks {
		t.Error("expected WithAdminLinks=false")
	}
	if filter.WithNonBookable {
		t.Error("expected WithNonBookable=false")
	}
	if !filter.WithAutopacks {
		t.Error("expected WithAutopacks=true (feature enabled)")
	}
}

func TestBuildFilterStage_AdminUser(t *testing.T) {
	dataSec := &mockDataSecRepo{}
	whiteLabel := &mockWhiteLabelRepo{}
	flags := feature.New(nil)

	stage := &BuildFilterStage{
		dataSecRepo:    dataSec,
		whiteLabelRepo: whiteLabel,
		features:       flags,
	}

	date, _ := time.Parse("2006-01-02", "2023-09-09")
	in := BuildFilterInput{
		ResolvedPlaces: ResolvedPlaces{
			FromPlaceID:    "100",
			ToPlaceID:      "200p",
			FromStationIDs: []int{100},
			ToStationIDs:   []int{200},
		},
		SearchParams: domain.SearchParams{
			WithNonBookable: true,
		},
		Agent: domain.AgentContext{
			IsAdmin: true,
		},
		Date: date,
	}

	filter, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !filter.WithAdminLinks {
		t.Error("expected WithAdminLinks=true for admin")
	}
	if !filter.WithNonBookable {
		t.Error("expected WithNonBookable=true for admin with param")
	}
}

func TestBuildFilterStage_BotRecheckLevel(t *testing.T) {
	dataSec := &mockDataSecRepo{}
	whiteLabel := &mockWhiteLabelRepo{}
	flags := feature.New(nil)

	stage := &BuildFilterStage{
		dataSecRepo:    dataSec,
		whiteLabelRepo: whiteLabel,
		features:       flags,
	}

	date, _ := time.Parse("2006-01-02", "2023-09-09")
	in := BuildFilterInput{
		ResolvedPlaces: ResolvedPlaces{
			FromStationIDs: []int{100},
			ToStationIDs:   []int{200},
		},
		Agent: domain.AgentContext{IsBot: true},
		Date:  date,
	}

	filter, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if filter.RecheckLevel != domain.RecheckNever {
		t.Errorf("RecheckLevel = %d, want %d (RecheckNever for bots)", filter.RecheckLevel, domain.RecheckNever)
	}
}

func TestBuildFilterStage_DataSecRestrictions(t *testing.T) {
	dataSec := &mockDataSecRepo{
		restrictions: repository.SecurityRestrictions{
			OperatorIDs:        []int{10, 20},
			ExcludeOperatorIDs: []int{99},
			CountryIDs:         []string{"TH", "VN"},
		},
	}
	whiteLabel := &mockWhiteLabelRepo{}
	flags := feature.New(nil)

	stage := &BuildFilterStage{
		dataSecRepo:    dataSec,
		whiteLabelRepo: whiteLabel,
		features:       flags,
	}

	date, _ := time.Parse("2006-01-02", "2023-09-09")
	in := BuildFilterInput{
		ResolvedPlaces: ResolvedPlaces{
			FromStationIDs: []int{100},
			ToStationIDs:   []int{200},
		},
		Agent: domain.AgentContext{AgentID: 42},
		Date:  date,
	}

	filter, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(filter.OperatorIDs) != 2 || filter.OperatorIDs[0] != 10 {
		t.Errorf("OperatorIDs = %v, want [10 20]", filter.OperatorIDs)
	}
	if len(filter.ExcludeOperatorIDs) != 1 || filter.ExcludeOperatorIDs[0] != 99 {
		t.Errorf("ExcludeOperatorIDs = %v, want [99]", filter.ExcludeOperatorIDs)
	}
	if len(filter.CountryIDs) != 2 {
		t.Errorf("CountryIDs = %v, want [TH VN]", filter.CountryIDs)
	}
}

func TestBuildFilterStage_WhiteLabelIntersection(t *testing.T) {
	dataSec := &mockDataSecRepo{
		restrictions: repository.SecurityRestrictions{
			OperatorIDs: []int{10, 20, 30},
		},
	}
	whiteLabel := &mockWhiteLabelRepo{
		config: repository.WhiteLabelConfig{
			OperatorIDs: []int{20, 30, 40},
		},
	}
	flags := feature.New(nil)

	stage := &BuildFilterStage{
		dataSecRepo:    dataSec,
		whiteLabelRepo: whiteLabel,
		features:       flags,
	}

	date, _ := time.Parse("2006-01-02", "2023-09-09")
	in := BuildFilterInput{
		ResolvedPlaces: ResolvedPlaces{
			FromStationIDs: []int{100},
			ToStationIDs:   []int{200},
		},
		Agent: domain.AgentContext{AgentID: 42},
		Date:  date,
	}

	filter, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Intersection of [10,20,30] and [20,30,40] = [20,30]
	if len(filter.OperatorIDs) != 2 {
		t.Errorf("OperatorIDs = %v, want intersection [20 30]", filter.OperatorIDs)
	}
}

func TestBuildFilterStage_WhiteLabelNoOverlap(t *testing.T) {
	dataSec := &mockDataSecRepo{
		restrictions: repository.SecurityRestrictions{
			OperatorIDs: []int{10},
		},
	}
	whiteLabel := &mockWhiteLabelRepo{
		config: repository.WhiteLabelConfig{
			OperatorIDs: []int{20},
		},
	}
	flags := feature.New(nil)

	stage := &BuildFilterStage{
		dataSecRepo:    dataSec,
		whiteLabelRepo: whiteLabel,
		features:       flags,
	}

	date, _ := time.Parse("2006-01-02", "2023-09-09")
	in := BuildFilterInput{
		ResolvedPlaces: ResolvedPlaces{
			FromStationIDs: []int{100},
			ToStationIDs:   []int{200},
		},
		Agent: domain.AgentContext{AgentID: 42},
		Date:  date,
	}

	filter, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No overlap → IsNotPossible
	if !filter.IsNotPossible {
		t.Error("expected IsNotPossible=true when WL and DataSec operators don't overlap")
	}
}

func TestBuildFilterStage_VehclassFromParam(t *testing.T) {
	dataSec := &mockDataSecRepo{}
	whiteLabel := &mockWhiteLabelRepo{}
	flags := feature.New(nil)

	stage := &BuildFilterStage{
		dataSecRepo:    dataSec,
		whiteLabelRepo: whiteLabel,
		features:       flags,
	}

	date, _ := time.Parse("2006-01-02", "2023-09-09")
	in := BuildFilterInput{
		ResolvedPlaces: ResolvedPlaces{
			FromStationIDs: []int{100},
			ToStationIDs:   []int{200},
		},
		SearchParams: domain.SearchParams{
			VehclassID: "999",
		},
		Date: date,
	}

	filter, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(filter.VehclassIDs) != 1 || filter.VehclassIDs[0] != "999" {
		t.Errorf("VehclassIDs = %v, want [999]", filter.VehclassIDs)
	}
}

func TestIntersectInts(t *testing.T) {
	tests := []struct {
		name string
		a, b []int
		want []int
	}{
		{"both empty", nil, nil, nil},
		{"a empty returns b", nil, []int{1, 2}, []int{1, 2}},
		{"b empty returns a", []int{1, 2}, nil, []int{1, 2}},
		{"no overlap", []int{1, 2}, []int{3, 4}, nil},
		{"full overlap", []int{1, 2, 3}, []int{1, 2, 3}, []int{1, 2, 3}},
		{"partial overlap", []int{1, 2, 3}, []int{2, 3, 4}, []int{2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intersectInts(tt.a, tt.b)
			if len(got) != len(tt.want) {
				t.Errorf("intersectInts(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("intersectInts(%v, %v)[%d] = %d, want %d", tt.a, tt.b, i, v, tt.want[i])
				}
			}
		})
	}
}
