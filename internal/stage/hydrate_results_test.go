package stage

import (
	"context"
	"testing"

	"github.com/12go/f4/internal/domain"
)

func TestHydrateResults_BasicTrip(t *testing.T) {
	stage := NewHydrateResultsStage()

	raw := domain.RawTrip{
		TripKey:         "TH-BKK-CNX-01",
		Duration:        720,
		DepartureTime:   480,
		OperatorID:      1,
		ClassID:         10,
		VehclassID:      "bus",
		DepStationID:    100,
		ArrStationID:    200,
		Dep:             "2024-01-01 08:00:00",
		Arr:             "2024-01-01 20:00:00",
		OpBookable:      true,
		IntegrationCode: "manual",
		Price: domain.TripPrice{
			IsValid:    true,
			PriceLevel: domain.PriceExact,
			Total:      500.00,
			Avail:      45,
		},
		RankScoreFormula: 85.5,
		SpecialDealFlag:  true,
		NewTripFlag:      true,
		Amenities:        "wifi,power",
		TicketType:       "eticket",
		BaggageFreeWeight: 20,
		IsFRefundable:    true,
	}

	in := EnrichedTrips{
		Trips:     []domain.RawTrip{raw},
		Operators: map[int]domain.Operator{1: {OperatorID: 1, Name: "TestOp"}},
		Stations:  map[int]domain.Station{100: {StationID: 100}, 200: {StationID: 200}},
		Classes:   map[int]domain.VehicleClass{10: {ClassID: 10}},
		Filter:    domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(out.Trips))
	}

	trip := out.Trips[0]

	if trip.TripKey != "TH-BKK-CNX-01" {
		t.Errorf("TripKey = %q, want %q", trip.TripKey, "TH-BKK-CNX-01")
	}
	if !trip.IsBookable {
		t.Error("expected IsBookable=true")
	}
	if !trip.HasValidPrice {
		t.Error("expected HasValidPrice=true")
	}
	if trip.RankScore != 85.5 {
		t.Errorf("RankScore = %f, want 85.5", trip.RankScore)
	}
	if !trip.SpecialDeal {
		t.Error("expected SpecialDeal=true")
	}
	if !trip.NewTrip {
		t.Error("expected NewTrip=true")
	}

	// Check segments
	if len(trip.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(trip.Segments))
	}
	seg := trip.Segments[0]
	if seg.FromStationID != 100 || seg.ToStationID != 200 {
		t.Errorf("segment stations = %d→%d, want 100→200", seg.FromStationID, seg.ToStationID)
	}
	if seg.Duration != 720 {
		t.Errorf("segment duration = %d, want 720", seg.Duration)
	}
	if seg.Type != "route" {
		t.Errorf("segment type = %q, want %q", seg.Type, "route")
	}

	// Check travel options
	if len(trip.TravelOptions) != 1 {
		t.Fatalf("expected 1 travel option, got %d", len(trip.TravelOptions))
	}
	opt := trip.TravelOptions[0]
	if opt.TripKey != "TH-BKK-CNX-01" {
		t.Errorf("option TripKey = %q", opt.TripKey)
	}
	if opt.IntegrationCode != "manual" {
		t.Errorf("option IntegrationCode = %q, want %q", opt.IntegrationCode, "manual")
	}
	if opt.AvailableSeats != 45 {
		t.Errorf("option AvailableSeats = %d, want 45", opt.AvailableSeats)
	}

	// Check tags
	expectedTags := []string{"wifi", "power", "ticket:eticket", "baggage:20kg", "refundable", "special_deal", "new"}
	if len(trip.Tags) != len(expectedTags) {
		t.Errorf("tags = %v, want %v", trip.Tags, expectedTags)
	}
}

func TestHydrateResults_ConnectionDetection(t *testing.T) {
	stage := NewHydrateResultsStage()

	// Trip with SetID → is connection
	setID := 42
	raw := domain.RawTrip{
		TripKey:      "conn1",
		SetID:        &setID,
		DepStationID: 100,
		ArrStationID: 200,
		OpBookable:   true,
		Price:        domain.TripPrice{IsValid: true},
	}

	in := EnrichedTrips{
		Trips:     []domain.RawTrip{raw},
		Operators: map[int]domain.Operator{},
		Stations:  map[int]domain.Station{},
		Classes:   map[int]domain.VehicleClass{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.Trips[0].IsConnection {
		t.Error("expected IsConnection=true for trip with SetID")
	}

	// Trip with Departure2Time > 0 → also a connection
	raw2 := domain.RawTrip{
		TripKey:        "conn2",
		Departure2Time: 600,
		DepStationID:   100,
		ArrStationID:   200,
		OpBookable:     true,
		Price:          domain.TripPrice{IsValid: true},
	}

	in2 := EnrichedTrips{
		Trips:     []domain.RawTrip{raw2},
		Operators: map[int]domain.Operator{},
		Stations:  map[int]domain.Station{},
		Classes:   map[int]domain.VehicleClass{},
	}

	out2, err := stage.Execute(context.Background(), in2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out2.Trips[0].IsConnection {
		t.Error("expected IsConnection=true for trip with Departure2Time > 0")
	}
}

func TestHydrateResults_GroupKey(t *testing.T) {
	stage := NewHydrateResultsStage()

	raw := domain.RawTrip{
		DepStationID:  100,
		ArrStationID:  200,
		DepartureTime: 480,
		Duration:      120,
		OpBookable:    true,
		Price:         domain.TripPrice{IsValid: true},
	}

	in := EnrichedTrips{
		Trips:     []domain.RawTrip{raw},
		Operators: map[int]domain.Operator{},
		Stations:  map[int]domain.Station{},
		Classes:   map[int]domain.VehicleClass{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "100-200-480-120"
	if out.Trips[0].GroupKey != expected {
		t.Errorf("GroupKey = %q, want %q", out.Trips[0].GroupKey, expected)
	}
}

func TestHydrateResults_EmptyInput(t *testing.T) {
	stage := NewHydrateResultsStage()

	in := EnrichedTrips{
		Operators: map[int]domain.Operator{},
		Stations:  map[int]domain.Station{},
		Classes:   map[int]domain.VehicleClass{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 0 {
		t.Errorf("expected 0 trips, got %d", len(out.Trips))
	}
}
