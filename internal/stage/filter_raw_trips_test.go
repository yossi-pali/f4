package stage

import (
	"context"
	"testing"

	"github.com/12go/f4/internal/domain"
)

func makeRawTrip(tripKey string, operatorID, depStation, arrStation int) domain.RawTrip {
	return domain.RawTrip{
		TripKey:      tripKey,
		OperatorID:   operatorID,
		DepStationID: depStation,
		ArrStationID: arrStation,
		OpBookable:   true,
	}
}

func TestFilterRawTrips_FilterHiddenDepartures(t *testing.T) {
	stage := NewFilterRawTripsStage()

	trip1 := makeRawTrip("trip1", 1, 100, 200)
	trip2 := makeRawTrip("trip2", 2, 100, 200)
	trip2.DepHideDeparture = true

	in := RawTripsResult{
		Trips:  []domain.RawTrip{trip1, trip2},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.DirectTrips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(out.DirectTrips))
	}
	if out.DirectTrips[0].TripKey != "trip1" {
		t.Errorf("expected trip1, got %s", out.DirectTrips[0].TripKey)
	}
}

func TestFilterRawTrips_FilterMetaOperators(t *testing.T) {
	stage := NewFilterRawTripsStage()

	trip1 := makeRawTrip("trip1", 1, 100, 200)
	trip2 := makeRawTrip("trip2", 2, 100, 200)
	trip2.IsMeta = true

	in := RawTripsResult{
		Trips:  []domain.RawTrip{trip1, trip2},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.DirectTrips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(out.DirectTrips))
	}
}

func TestFilterRawTrips_SeparateConnections(t *testing.T) {
	stage := NewFilterRawTripsStage()

	trip1 := makeRawTrip("trip1", 1, 100, 200)
	trip2 := makeRawTrip("trip2", 2, 100, 200)
	setID := 42
	trip2.SetID = &setID

	in := RawTripsResult{
		Trips:  []domain.RawTrip{trip1, trip2},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.DirectTrips) != 1 {
		t.Fatalf("expected 1 direct trip, got %d", len(out.DirectTrips))
	}
	if len(out.ConnectionIDs) != 1 || out.ConnectionIDs[0] != 42 {
		t.Errorf("expected ConnectionIDs=[42], got %v", out.ConnectionIDs)
	}
}

func TestFilterRawTrips_DaytripDetection(t *testing.T) {
	stage := NewFilterRawTripsStage()

	// trip1: A→B by operator 1
	trip1 := makeRawTrip("trip1", 1, 100, 200)
	// trip2: B→A by operator 1 (reverse of trip1, same operator = daytrip)
	trip2 := makeRawTrip("trip2", 1, 200, 100)
	// trip3: B→A by operator 2 (different operator, not a daytrip)
	trip3 := makeRawTrip("trip3", 2, 200, 100)

	in := RawTripsResult{
		Trips:  []domain.RawTrip{trip1, trip2, trip3},
		Filter: domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.DirectTrips) != 2 {
		t.Fatalf("expected 2 trips (trip2 removed as daytrip), got %d", len(out.DirectTrips))
	}
	for _, trip := range out.DirectTrips {
		if trip.TripKey == "trip2" {
			t.Error("trip2 should have been filtered as daytrip")
		}
	}
}

func TestFilterRawTrips_OnlyPairs(t *testing.T) {
	stage := NewFilterRawTripsStage()

	trip1 := makeRawTrip("trip1", 1, 100, 200) // matching pair
	trip2 := makeRawTrip("trip2", 2, 100, 300) // non-matching to station

	in := RawTripsResult{
		Trips: []domain.RawTrip{trip1, trip2},
		Filter: domain.SearchFilter{
			OnlyPairs:      true,
			FromStationIDs: []int{100},
			ToStationIDs:   []int{200},
		},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.DirectTrips) != 1 {
		t.Fatalf("expected 1 trip (only matching pair), got %d", len(out.DirectTrips))
	}
	if out.DirectTrips[0].TripKey != "trip1" {
		t.Errorf("expected trip1, got %s", out.DirectTrips[0].TripKey)
	}
}

func TestFilterRawTrips_EmptyInput(t *testing.T) {
	stage := NewFilterRawTripsStage()

	in := RawTripsResult{Filter: domain.SearchFilter{}}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.DirectTrips) != 0 {
		t.Errorf("expected 0 trips, got %d", len(out.DirectTrips))
	}
}

func TestIsInStationPairs(t *testing.T) {
	tests := []struct {
		name            string
		fromID, toID    int
		fromIDs, toIDs  []int
		want            bool
	}{
		{"matching pair", 100, 200, []int{100, 101}, []int{200, 201}, true},
		{"from not in list", 999, 200, []int{100, 101}, []int{200, 201}, false},
		{"to not in list", 100, 999, []int{100, 101}, []int{200, 201}, false},
		{"both not matching", 999, 888, []int{100}, []int{200}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInStationPairs(tt.fromID, tt.toID, tt.fromIDs, tt.toIDs)
			if got != tt.want {
				t.Errorf("isInStationPairs(%d, %d, ...) = %v, want %v", tt.fromID, tt.toID, got, tt.want)
			}
		})
	}
}
