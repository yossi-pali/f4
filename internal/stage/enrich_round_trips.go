package stage

import (
	"context"

	"github.com/12go/f4/internal/db"
	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/event"
	"github.com/12go/f4/internal/price"
	"github.com/12go/f4/internal/repository"
)

// EnrichRoundTripsInput is the input for Stage 5b.
type EnrichRoundTripsInput struct {
	DirectTrips []domain.RawTrip
	Filter      domain.SearchFilter
}

// RoundTripEnrichedTrips is the output of Stage 5b.
type RoundTripEnrichedTrips struct {
	Trips []domain.RawTrip
}

// roundTripPriceFinder is a local interface for round trip price lookups.
type roundTripPriceFinder interface {
	FindByOutbound(ctx context.Context, region, outboundTripKey string, outboundGodate string) ([]repository.RoundTripPriceRow, error)
}

// EnrichRoundTripsStage applies round trip discounts to inbound trips.
type EnrichRoundTripsStage struct {
	roundTripPriceRepo roundTripPriceFinder
	publisher          event.Publisher
	regionResolver     db.RegionResolver
}

func NewEnrichRoundTripsStage(
	roundTripPriceRepo roundTripPriceFinder,
	publisher event.Publisher,
	regionResolver db.RegionResolver,
) *EnrichRoundTripsStage {
	return &EnrichRoundTripsStage{
		roundTripPriceRepo: roundTripPriceRepo,
		publisher:          publisher,
		regionResolver:     regionResolver,
	}
}

func (s *EnrichRoundTripsStage) Name() string { return "enrich_round_trips" }

func (s *EnrichRoundTripsStage) Execute(ctx context.Context, in EnrichRoundTripsInput) (RoundTripEnrichedTrips, error) {
	// If no outbound trip specified, pass through unchanged
	if in.Filter.OutboundTrip == nil {
		return RoundTripEnrichedTrips{Trips: in.DirectTrips}, nil
	}

	outbound := in.Filter.OutboundTrip
	region := db.DefaultRegion
	if len(in.Filter.FromStationIDs) > 0 {
		region = s.regionResolver.ResolveByStationID(in.Filter.FromStationIDs[0])
	}

	godateStr := in.Filter.Date.Format("2006-01-02")

	// Look up cached round trip prices
	rtPrices, err := s.roundTripPriceRepo.FindByOutbound(ctx, region, outbound.TripKey, godateStr)
	if err != nil {
		// Non-fatal: continue without round trip enrichment
		return RoundTripEnrichedTrips{Trips: in.DirectTrips}, nil
	}

	if len(rtPrices) == 0 {
		// Cache miss: emit event for Integration Service to populate
		_ = s.publisher.Publish(ctx, domain.TopicSearchNeedsRoundTrip, domain.SearchNeedsRoundTripPricesEvent{
			OutboundTripKey: outbound.TripKey,
			OutboundGodate:  godateStr,
			InboundDate:     in.Filter.Date.Format("2006-01-02"),
			IntegrationCode: outbound.IntegrationCode,
			SeatsAdult:      in.Filter.SeatsAdult,
			SeatsChild:      in.Filter.SeatsChild,
			SeatsInfant:     in.Filter.SeatsInfant,
		})
		return RoundTripEnrichedTrips{Trips: in.DirectTrips}, nil
	}

	// Index round trip prices by inbound trip key + departure time
	type rtKey struct {
		tripKey       string
		departureTime int
	}
	rtIndex := make(map[rtKey]repository.RoundTripPriceRow, len(rtPrices))
	for _, rtp := range rtPrices {
		key := rtKey{rtp.InboundTripKey, rtp.InboundDepartureTime}
		rtIndex[key] = rtp
	}

	// Apply round trip discounts to matching inbound trips
	trips := make([]domain.RawTrip, len(in.DirectTrips))
	copy(trips, in.DirectTrips)

	for i := range trips {
		key := rtKey{trips[i].TripKey, trips[i].DepartureTime}
		if rtPrice, ok := rtIndex[key]; ok {
			// Decode round trip price
			if len(rtPrice.PriceBinStr) > 0 {
				rtDecoded, err := price.Decode(rtPrice.PriceBinStr)
				if err == nil && rtDecoded.Total > 0 && outbound.Price.Total > 0 && trips[i].Price.Total > 0 {
					originalTotal := outbound.Price.Total + trips[i].Price.Total
					if originalTotal > 0 {
						discountPct := (originalTotal - rtDecoded.Total) / originalTotal * 100
						trips[i].RoundTripDiscountPct = discountPct
						trips[i].Price.Total = rtDecoded.Total - outbound.Price.Total
						if trips[i].Price.Total < 0 {
							trips[i].Price.Total = 0
						}
					}
				}
			}
		}
	}

	return RoundTripEnrichedTrips{Trips: trips}, nil
}
