package stage

import (
	"context"
	"testing"
	"time"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/event"
	"github.com/12go/f4/internal/repository"
)

// mockRoundTripPriceRepo is a test mock for RoundTripPriceRepo.
type mockRoundTripPriceRepo struct {
	prices []repository.RoundTripPriceRow
	err    error
}

func (m *mockRoundTripPriceRepo) FindByOutbound(_ context.Context, _ string, _ string, _ string) ([]repository.RoundTripPriceRow, error) {
	return m.prices, m.err
}

// mockRegionResolver always returns "th".
type mockRegionResolver struct{}

func (m *mockRegionResolver) ResolveByStationID(_ int) string    { return "th" }
func (m *mockRegionResolver) ResolveByPlaceID(_ string) string   { return "th" }
func (m *mockRegionResolver) ResolveByCountryID(_ string) string { return "th" }
func (m *mockRegionResolver) ResolveByTripKey(_ string) string   { return "th" }

func TestEnrichRoundTrips_NoOutboundTrip(t *testing.T) {
	stage := &EnrichRoundTripsStage{
		roundTripPriceRepo: &mockRoundTripPriceRepo{},
		publisher:          &event.NoopPublisher{},
		regionResolver:     &mockRegionResolver{},
	}

	trips := []domain.RawTrip{
		{TripKey: "trip1", Price: domain.TripPrice{Total: 100}},
	}

	in := EnrichRoundTripsInput{
		DirectTrips: trips,
		Filter:      domain.SearchFilter{},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 1 || out.Trips[0].TripKey != "trip1" {
		t.Errorf("expected trips to pass through unchanged, got %v", out.Trips)
	}
}

func TestEnrichRoundTrips_CacheMissEmitsEvent(t *testing.T) {
	publishedTopics := make([]string, 0)
	mockPub := &trackingPublisher{topics: &publishedTopics}

	stage := &EnrichRoundTripsStage{
		roundTripPriceRepo: &mockRoundTripPriceRepo{prices: nil},
		publisher:          mockPub,
		regionResolver:     &mockRegionResolver{},
	}

	date, _ := time.Parse("2006-01-02", "2024-01-01")
	in := EnrichRoundTripsInput{
		DirectTrips: []domain.RawTrip{{TripKey: "inbound1", DepartureTime: 600, Price: domain.TripPrice{Total: 50}}},
		Filter: domain.SearchFilter{
			OutboundTrip:   &domain.TripPlain{TripKey: "outbound1", IntegrationCode: "manual"},
			FromStationIDs: []int{100},
			Date:           date,
			SeatsAdult:     1,
		},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Trips should pass through unchanged
	if len(out.Trips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(out.Trips))
	}

	// Should have published an event for round trip price lookup
	if len(publishedTopics) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publishedTopics))
	}
	if publishedTopics[0] != domain.TopicSearchNeedsRoundTrip {
		t.Errorf("expected topic %q, got %q", domain.TopicSearchNeedsRoundTrip, publishedTopics[0])
	}
}

func TestEnrichRoundTrips_DiscountCalculation(t *testing.T) {
	// Ported from PHP RoundTripPoolPriceManagerTest::testCalculateDiscountPercentByPrices
	// roundPrice=100 per adult, outbound=100, inbound=100 → original=200, roundTrip=100 → discount=50%
	stage := &EnrichRoundTripsStage{
		roundTripPriceRepo: &mockRoundTripPriceRepo{
			prices: []repository.RoundTripPriceRow{
				{
					InboundTripKey:       "inbound1",
					InboundDepartureTime: 600,
					PriceBinStr:          makeTestPriceBytes(100_00), // 100.00
				},
			},
		},
		publisher:      &event.NoopPublisher{},
		regionResolver: &mockRegionResolver{},
	}

	date, _ := time.Parse("2006-01-02", "2024-01-01")
	in := EnrichRoundTripsInput{
		DirectTrips: []domain.RawTrip{
			{
				TripKey:       "inbound1",
				DepartureTime: 600,
				Price:         domain.TripPrice{IsValid: true, Total: 80},
			},
		},
		Filter: domain.SearchFilter{
			OutboundTrip: &domain.TripPlain{
				TripKey: "outbound1",
				Price:   domain.TripPrice{IsValid: true, Total: 60},
			},
			FromStationIDs: []int{100},
			Date:           date,
		},
	}

	out, err := stage.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Trips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(out.Trips))
	}

	trip := out.Trips[0]

	// Round trip total = 100.00, outbound price = 60.00
	// Discounted inbound = 100 - 60 = 40.00
	if trip.Price.Total != 40.00 {
		t.Errorf("expected discounted inbound price=40.00, got %f", trip.Price.Total)
	}

	// originalTotal = outbound(60) + inbound(80) = 140
	// discountPct = (140 - 100) / 140 * 100 ≈ 28.57%
	expectedPct := (140.0 - 100.0) / 140.0 * 100.0
	if trip.RoundTripDiscountPct < expectedPct-0.01 || trip.RoundTripDiscountPct > expectedPct+0.01 {
		t.Errorf("expected discount pct≈%.2f%%, got %.2f%%", expectedPct, trip.RoundTripDiscountPct)
	}
}

// trackingPublisher records published topics.
type trackingPublisher struct {
	topics *[]string
}

func (p *trackingPublisher) Publish(_ context.Context, topic string, _ any) error {
	*p.topics = append(*p.topics, topic)
	return nil
}

func (p *trackingPublisher) Close() error { return nil }

// makeTestPriceBytes creates a minimal 112-byte price binary with the given total (in cents).
// Header layout (17 bytes): flags(1)+avail(1)+level(1)+reason_id(1)+reason_param(4)+stamp(4)+fxid(1)+total(4)
func makeTestPriceBytes(totalCents uint32) []byte {
	data := make([]byte, 112)
	data[0] = 0x01 // flagValid
	data[2] = byte(domain.PriceExact)
	data[12] = 4 // fxid = USD (offset 12)
	// total at offset 13-16 (little-endian uint32)
	data[13] = byte(totalCents)
	data[14] = byte(totalCents >> 8)
	data[15] = byte(totalCents >> 16)
	data[16] = byte(totalCents >> 24)
	return data
}
