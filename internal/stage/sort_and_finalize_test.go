package stage

import (
	"context"
	"testing"

	"github.com/12go/f4/internal/domain"
)

// mockStationRepoForFinalize is a mock for the GetParentProvinceName dependency.
type mockStationRepoForFinalize struct {
	provinceName string
}

func (m *mockStationRepoForFinalize) GetParentProvinceName(_ context.Context, _ string) string {
	return m.provinceName
}

func makeTripResult(tripKey string, bookable, validPrice bool, rankScore float64, integrationCode string) domain.TripResult {
	seats := 0
	if bookable {
		seats = 10
	}
	return domain.TripResult{
		TripKey:       tripKey,
		GroupKey:      tripKey, // unique group key for simplicity
		IsBookable:    bookable,
		HasValidPrice: validPrice,
		RankScore:     rankScore,
		TravelOptions: []domain.TravelOption{
			{
				TripKey:         tripKey,
				IntegrationCode: integrationCode,
				UniqueKey:       tripKey + "-opt1",
				Bookable:        seats,
				Price: domain.TripPrice{
					IsValid:    validPrice,
					PriceLevel: domain.PriceExact,
				},
			},
		},
	}
}

func TestSortAndFinalize_SortByRankScore(t *testing.T) {
	// Matches PHP ChiefCookTest::testSortSearchResults
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})
	in := HydratedResults{
		Trips: []domain.TripResult{
			makeTripResult("trip1", true, true, 99, "int1"),
			makeTripResult("trip2", true, true, 999, "int2"),
		},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 2 {
		t.Fatalf("expected 2 trips, got %d", len(out.Trips))
	}
	// Higher rank score should come first
	if out.Trips[0].TripKey != "trip2" {
		t.Errorf("expected trip2 first (rankScore=999), got %s", out.Trips[0].TripKey)
	}
	if out.Trips[1].TripKey != "trip1" {
		t.Errorf("expected trip1 second (rankScore=99), got %s", out.Trips[1].TripKey)
	}
}

func TestSortAndFinalize_BookableBeforeNonBookable(t *testing.T) {
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})
	in := HydratedResults{
		Trips: []domain.TripResult{
			makeTripResult("notbookable", false, true, 100, "int1"),
			makeTripResult("bookable", true, true, 50, "int2"),
		},
		Filter: domain.SearchFilter{WithNonBookable: true, WithAdminLinks: true},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 2 {
		t.Fatalf("expected 2 trips, got %d", len(out.Trips))
	}
	if out.Trips[0].TripKey != "bookable" {
		t.Error("expected bookable trip to come first")
	}
}

func TestSortAndFinalize_FilterNonBookable(t *testing.T) {
	// Matches PHP testFinishCookOldFormatWhenOptionUnavailable behavior
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})
	in := HydratedResults{
		Trips: []domain.TripResult{
			makeTripResult("notbookable", false, true, 100, "int1"),
			makeTripResult("bookable", true, true, 50, "int2"),
		},
		Filter: domain.SearchFilter{WithNonBookable: false},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 1 {
		t.Fatalf("expected 1 trip (non-bookable filtered), got %d", len(out.Trips))
	}
	if out.Trips[0].TripKey != "bookable" {
		t.Errorf("expected bookable trip, got %s", out.Trips[0].TripKey)
	}
}

func TestSortAndFinalize_FilterInvalidPrice(t *testing.T) {
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})
	in := HydratedResults{
		Trips: []domain.TripResult{
			makeTripResult("noprice", true, false, 100, "int1"),
			makeTripResult("withprice", true, true, 50, "int2"),
		},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 1 {
		t.Fatalf("expected 1 trip (invalid price filtered), got %d", len(out.Trips))
	}
	if out.Trips[0].TripKey != "withprice" {
		t.Errorf("expected withprice trip, got %s", out.Trips[0].TripKey)
	}
}

func TestSortAndFinalize_PresentIntegrations(t *testing.T) {
	// Matches PHP ChiefCookTest: presentIntegration should contain all integration codes
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})
	in := HydratedResults{
		Trips: []domain.TripResult{
			makeTripResult("trip1", true, true, 100, "integration0"),
			makeTripResult("trip2", true, true, 100, "integration1"),
			makeTripResult("trip3", true, true, 100, "integration0"), // duplicate
		},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have exactly 2 unique integration codes
	if len(out.PresentIntegrations) != 2 {
		t.Errorf("expected 2 present integrations, got %d: %v", len(out.PresentIntegrations), out.PresentIntegrations)
	}
}

func TestSortAndFinalize_MergeDuplicates(t *testing.T) {
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})

	// Two trips with the same GroupKey should be merged
	trip1 := makeTripResult("trip1", true, true, 100, "int1")
	trip1.GroupKey = "same-key"

	trip2 := makeTripResult("trip2", true, true, 200, "int2")
	trip2.GroupKey = "same-key"

	in := HydratedResults{
		Trips:  []domain.TripResult{trip1, trip2},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 1 {
		t.Fatalf("expected 1 merged trip, got %d", len(out.Trips))
	}
	// Should have travel options from both
	if len(out.Trips[0].TravelOptions) != 2 {
		t.Errorf("expected 2 travel options after merge, got %d", len(out.Trips[0].TravelOptions))
	}
	// RankScore should be the best (200)
	if out.Trips[0].RankScore != 200 {
		t.Errorf("expected RankScore=200, got %f", out.Trips[0].RankScore)
	}
}

func TestSortAndFinalize_DedupTravelOptions(t *testing.T) {
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})

	trip := makeTripResult("trip1", true, true, 100, "int1")
	// Add a duplicate travel option with the same UniqueKey
	trip.TravelOptions = append(trip.TravelOptions, domain.TravelOption{
		TripKey:         "trip1",
		IntegrationCode: "int1",
		UniqueKey:       "trip1-opt1", // same key as first option
		Price:           domain.TripPrice{IsValid: true, PriceLevel: domain.PriceExact},
	})

	in := HydratedResults{
		Trips:  []domain.TripResult{trip},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(out.Trips))
	}
	if len(out.Trips[0].TravelOptions) != 1 {
		t.Errorf("expected 1 travel option after dedup, got %d", len(out.Trips[0].TravelOptions))
	}
}

func TestSortAndFinalize_RecheckKeys(t *testing.T) {
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})

	// PHP ChiefCook: travel options with IsValid=false (no price binary or invalid price)
	// go to recheck URLs, not main results.
	trip := domain.TripResult{
		TripKey:       "trip1",
		GroupKey:      "trip1",
		IsBookable:    false,
		HasValidPrice: false,
		TravelOptions: []domain.TravelOption{
			{
				TripKey:         "trip1",
				IntegrationCode: "int1",
				UniqueKey:       "trip1-opt1",
				Price:           domain.TripPrice{IsValid: false, PriceLevel: domain.PriceExact},
			},
		},
	}

	in := HydratedResults{
		Trips:  []domain.TripResult{trip},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.RecheckTripKeys) != 1 {
		t.Errorf("expected 1 recheck key, got %d", len(out.RecheckTripKeys))
	}
}

func TestSortAndFinalize_EmptyInput(t *testing.T) {
	stage := NewSortAndFinalizeStage(&mockStationRepoForFinalize{})

	in := HydratedResults{
		Trips:  nil,
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Trips == nil || len(out.Trips) != 0 {
		t.Errorf("expected empty non-nil trips slice, got %v", out.Trips)
	}
}

func TestDedupTravelOptions(t *testing.T) {
	opts := []domain.TravelOption{
		{UniqueKey: "a"},
		{UniqueKey: "b"},
		{UniqueKey: "a"},
		{UniqueKey: "c"},
		{UniqueKey: "b"},
	}

	result := dedupTravelOptions(opts)
	if len(result) != 3 {
		t.Errorf("expected 3 unique options, got %d", len(result))
	}
	expected := []string{"a", "b", "c"}
	for i, opt := range result {
		if opt.UniqueKey != expected[i] {
			t.Errorf("result[%d].UniqueKey = %q, want %q", i, opt.UniqueKey, expected[i])
		}
	}
}
