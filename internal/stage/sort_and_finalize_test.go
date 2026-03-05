package stage

import (
	"context"
	"testing"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/pipeline"
)

func TestSortAndFinalize_SortByRankScore(t *testing.T) {
	// Matches PHP ChiefCookTest::testSortSearchResults
	stage := NewSortAndFinalizeStage()
	pc := pipeline.NewPipelineContext("")
	ctx := pipeline.WithPipelineContext(context.Background(), pc)

	in := MergedResults{
		Trips: []domain.TripResult{
			{TripKey: "trip1", IsBookable: true, HasValidPrice: true, RankScore: 99},
			{TripKey: "trip2", IsBookable: true, HasValidPrice: true, RankScore: 999},
		},
	}

	out, err := stage.Execute(ctx, in)
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
	stage := NewSortAndFinalizeStage()
	pc := pipeline.NewPipelineContext("")
	ctx := pipeline.WithPipelineContext(context.Background(), pc)

	in := MergedResults{
		Trips: []domain.TripResult{
			{TripKey: "notbookable", IsBookable: false, HasValidPrice: true, RankScore: 100},
			{TripKey: "bookable", IsBookable: true, HasValidPrice: true, RankScore: 50},
		},
	}

	out, err := stage.Execute(ctx, in)
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
