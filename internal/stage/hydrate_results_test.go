package stage

import (
	"context"
	"math"
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

	// Tags: PHP builds tags from segment-level route features, not trip-level amenities.
	// Go returns empty tags to match PHP API v1 behavior.
	if len(trip.Tags) != 0 {
		t.Errorf("tags = %v, want empty", trip.Tags)
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
		OperatorID:    5,
		ClassID:       12,
		VehclassID:    "bus",
		OfficialID:    "TH1",
		Dep:           "2026-03-23 08:00:00",
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

	// PHP-style GroupKey: "{official_id}#{operator_id}#{vehclass_id}#{dep}#{arr}#{date 00:00:00+classID}#{setID}"
	// For non-connection trips with is_ignore_group_time=false, dep time is zeroed + classID appended
	expected := "TH1#5#bus#100#200#2026-03-23 00:00:0012#"
	if out.Trips[0].GroupKey != expected {
		t.Errorf("GroupKey = %q, want %q", out.Trips[0].GroupKey, expected)
	}
}

func TestHydrateResults_PHPMatchingFields(t *testing.T) {
	stage := NewHydrateResultsStage()

	raw := domain.RawTrip{
		TripKey:            "TH000300044mDt00c01hdkRw",
		Duration:           720,
		DepartureTime:      480,
		OperatorID:         16779,
		ClassID:            12,
		VehclassID:         "train",
		OfficialID:         "9",
		DepStationID:       3,
		ArrStationID:       4,
		Dep:                "2026-02-06 18:57:00",
		Arr:                "07.02.2026 07:15:00",
		ChunkKey:           "",
		OpBookable:         true,
		IntegrationCode:    "srt",
		IntegrationID:      127,
		RankScoreFormula:       18457,
		RankScoreSales:         7.42,
		RankScoreSalesReal90:   10.0,
		SalesPerMonth:      15,
		Bookings30d:        15,
		Bookings30dSolo:    3,
		RatingAvg:          4.5,
		RatingCount:        303,
		Amenities:          "aircon,steward,wc",
		TicketType:         "default",
		ConfirmMinutes:     0,
		AvgConfirmTime:     0,
		CancelHours:        0,
		IsFRefundable:      false,
		BaggageFreeWeight:  0,
		Price: domain.TripPrice{
			IsValid:    true,
			PriceLevel: domain.PriceExact,
			Total:      1133,
			FXCode:     "THB",
			Avail:      46,
		},
	}

	in := EnrichedTrips{
		Trips:     []domain.RawTrip{raw},
		Operators: map[int]domain.Operator{16779: {OperatorID: 16779, Name: "Thai Railway"}},
		Stations:  map[int]domain.Station{3: {StationID: 3}, 4: {StationID: 4}},
		Classes:   map[int]domain.VehicleClass{12: {ClassID: 12}},
		Filter:    domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trip := out.Trips[0]

	// Trip-level PHP fields
	if trip.ChunkKey != "127" {
		t.Errorf("ChunkKey = %q, want %q", trip.ChunkKey, "127")
	}
	if trip.RouteName != "route name" {
		t.Errorf("RouteName = %q, want %q", trip.RouteName, "route name")
	}
	if !trip.ShowMap {
		t.Error("expected ShowMap=true")
	}
	if trip.TransferID == "" {
		t.Error("expected non-empty TransferID")
	}
	// ScoreSorting = calculateRankScoreBySales: 7.42 * 1.0 * 100 = 742
	if trip.ScoreSorting != 742 {
		t.Errorf("ScoreSorting = %f, want 742", trip.ScoreSorting)
	}
	// SalesSorting = calculateRankSales: log(Bookings30d * 40) = log(15 * 40) = log(600)
	expectedSales := math.Log(600)
	if math.Abs(trip.SalesSorting-expectedSales) > 0.01 {
		t.Errorf("SalesSorting = %f, want %f", trip.SalesSorting, expectedSales)
	}
	if trip.BookingsLastMonth != 15 {
		t.Errorf("BookingsLastMonth = %d, want 15", trip.BookingsLastMonth)
	}
	if trip.IsSoloTraveler {
		t.Error("expected IsSoloTraveler=false (15 total, 3 solo)")
	}

	// Params
	if trip.ParamsFrom != 3 || trip.ParamsTo != 4 {
		t.Errorf("Params from/to = %d/%d, want 3/4", trip.ParamsFrom, trip.ParamsTo)
	}
	if trip.ParamsBookable != 46 {
		t.Errorf("ParamsBookable = %d, want 46", trip.ParamsBookable)
	}
	if trip.ParamsMinPrice == nil {
		t.Fatal("expected ParamsMinPrice to be set")
	}
	if trip.ParamsMinPrice.Value != 1133 {
		t.Errorf("ParamsMinPrice.Value = %f, want 1133", trip.ParamsMinPrice.Value)
	}

	// Segment
	seg := trip.Segments[0]
	if seg.TripID != "TH000300044mDt00c01hdkRw" {
		t.Errorf("seg.TripID = %q", seg.TripID)
	}
	if seg.OfficialID != "9" {
		t.Errorf("seg.OfficialID = %q, want %q", seg.OfficialID, "9")
	}
	if len(seg.Vehclasses) != 1 || seg.Vehclasses[0] != "train" {
		t.Errorf("seg.Vehclasses = %v, want [train]", seg.Vehclasses)
	}

	// TravelOption
	opt := trip.TravelOptions[0]
	if opt.ID != "TH000300044mDt00c01hdkRw" {
		t.Errorf("opt.ID = %q", opt.ID)
	}
	if opt.ClassID != 12 {
		t.Errorf("opt.ClassID = %d, want 12", opt.ClassID)
	}
	if opt.Bookable != 46 {
		t.Errorf("opt.Bookable = %d, want 46", opt.Bookable)
	}
	if opt.Rating == nil || *opt.Rating != 4.5 {
		t.Errorf("opt.Rating = %v, want 4.5", opt.Rating)
	}
	if opt.RatingCount == nil || *opt.RatingCount != 303 {
		t.Errorf("opt.RatingCount = %v, want 303", opt.RatingCount)
	}
	if len(opt.Amenities) != 3 {
		t.Errorf("opt.Amenities = %v, want [aircon steward wc]", opt.Amenities)
	}
	if opt.TicketType != "default" {
		t.Errorf("opt.TicketType = %q, want %q", opt.TicketType, "default")
	}
	if opt.CancellationMsg != "No refunds, no cancelation" {
		t.Errorf("opt.CancellationMsg = %q", opt.CancellationMsg)
	}
	if opt.IsBookable != 1 {
		t.Errorf("opt.IsBookable = %d, want 1", opt.IsBookable)
	}
	if len(opt.Buy) != 1 {
		t.Fatalf("opt.Buy len = %d, want 1", len(opt.Buy))
	}
	if opt.Buy[0].TripID != "TH000300044mDt00c01hdkRw" {
		t.Errorf("buy.TripID = %q", opt.Buy[0].TripID)
	}
	if opt.Buy[0].Date != "2026-02-06-18-57-00" {
		t.Errorf("buy.Date = %q, want %q", opt.Buy[0].Date, "2026-02-06-18-57-00")
	}
	if opt.BookingURL == "" {
		t.Error("expected non-empty BookingURL")
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
