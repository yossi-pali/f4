package response

import "github.com/12go/f4/internal/domain"

// SearchResultsV2 is the API v2 response, matching PHP SearchResults.
// V2 does not include station/operator/class dictionaries.
type SearchResultsV2 struct {
	Trips          []TripResultV2 `json:"trips"`
	Recheck        []string       `json:"recheck"`
	ToProvinceName string         `json:"toProvinceName,omitempty"`
}

// TripResultV2 is the API v2 trip representation with inline station/operator data.
type TripResultV2 struct {
	TripKey       string            `json:"trip_key"`
	GroupKey      string            `json:"group_key"`
	Segments      []SegmentV2       `json:"segments"`
	TravelOptions []TravelOptionV2  `json:"travel_options"`
	Tags          []string          `json:"tags,omitempty"`
	IsBookable    bool              `json:"is_bookable"`
	HasValidPrice bool              `json:"has_valid_price"`
	IsConnection  bool              `json:"is_connection"`
	RankScore     float64           `json:"rank_score"`
	SpecialDeal   bool              `json:"special_deal,omitempty"`
	NewTrip       bool              `json:"new_trip,omitempty"`
}

// SegmentV2 includes inline station data (no dictionary lookup needed by client).
type SegmentV2 struct {
	FromStationID int    `json:"from_station_id"`
	ToStationID   int    `json:"to_station_id"`
	Departure     string `json:"departure"`
	Arrival       string `json:"arrival"`
	Duration      int    `json:"duration"`
	OperatorID    int    `json:"operator_id"`
	ClassID       int    `json:"class_id"`
	VehclassID    string `json:"vehclass_id"`
	Type          string `json:"type"`
}

// TravelOptionV2 includes detailed price with deltas.
type TravelOptionV2 struct {
	TripKey         string    `json:"trip_key"`
	IntegrationCode string   `json:"integration_code"`
	Price           PriceV2  `json:"price"`
	AvailableSeats  int      `json:"available_seats"`
	DepartureTime   int      `json:"departure_time"`
}

// PriceV2 includes discount and rule deltas.
type PriceV2 struct {
	Total          float64                    `json:"total"`
	FXCode         string                     `json:"fxcode"`
	PriceLevel     int                        `json:"price_level"`
	IsValid        bool                       `json:"is_valid"`
	Fares          map[string]FareV2          `json:"fares,omitempty"`
	DiscountDeltas map[string]PriceDeltaV2    `json:"discount_deltas,omitempty"`
	RuleDeltas     map[string]PriceDeltaV2    `json:"rule_deltas,omitempty"`
}

// FareV2 is a single fare block in API v2.
type FareV2 struct {
	FullPrice float64 `json:"full_price"`
	FXCode    string  `json:"fxcode"`
	NetPrice  float64 `json:"net_price,omitempty"`
	Topup     float64 `json:"topup,omitempty"`
	SysFee    float64 `json:"sys_fee,omitempty"`
}

// PriceDeltaV2 represents a price adjustment.
type PriceDeltaV2 struct {
	ID            int     `json:"id"`
	TotalDelta    float64 `json:"total_delta"`
	NetPriceDelta float64 `json:"net_price_delta"`
	TopupDelta    float64 `json:"topup_delta"`
	SysFeeDelta   float64 `json:"sys_fee_delta"`
	AgFeeDelta    float64 `json:"ag_fee_delta"`
}

// FromDomainV2 converts domain types to API v2 response.
func FromDomainV2(trips []domain.TripResult, recheck []string, provinceName string) SearchResultsV2 {
	v2Trips := make([]TripResultV2, 0, len(trips))
	for _, t := range trips {
		v2Trips = append(v2Trips, tripToV2(t))
	}

	if recheck == nil {
		recheck = []string{}
	}

	return SearchResultsV2{
		Trips:          v2Trips,
		Recheck:        recheck,
		ToProvinceName: provinceName,
	}
}

func tripToV2(t domain.TripResult) TripResultV2 {
	segments := make([]SegmentV2, 0, len(t.Segments))
	for _, s := range t.Segments {
		segments = append(segments, SegmentV2{
			FromStationID: s.FromStationID,
			ToStationID:   s.ToStationID,
			Departure:     s.Departure,
			Arrival:       s.Arrival,
			Duration:      s.Duration,
			OperatorID:    s.OperatorID,
			ClassID:       s.ClassID,
			VehclassID:    s.VehclassID,
			Type:          s.Type,
		})
	}

	options := make([]TravelOptionV2, 0, len(t.TravelOptions))
	for _, o := range t.TravelOptions {
		options = append(options, travelOptionToV2(o))
	}

	return TripResultV2{
		TripKey:       t.TripKey,
		GroupKey:      t.GroupKey,
		Segments:      segments,
		TravelOptions: options,
		Tags:          t.Tags,
		IsBookable:    t.IsBookable,
		HasValidPrice: t.HasValidPrice,
		IsConnection:  t.IsConnection,
		RankScore:     t.RankScore,
		SpecialDeal:   t.SpecialDeal,
		NewTrip:       t.NewTrip,
	}
}

func travelOptionToV2(o domain.TravelOption) TravelOptionV2 {
	fares := make(map[string]FareV2)
	for name, f := range o.Price.Fares {
		if f != nil {
			fares[name] = FareV2{
				FullPrice: f.FullPrice,
				FXCode:    f.FullPriceFXCode,
				NetPrice:  f.NetPrice,
				Topup:     f.Topup,
				SysFee:    f.SysFee,
			}
		}
	}

	discountDeltas := make(map[string]PriceDeltaV2)
	for name, d := range o.Price.DiscountDelta {
		if d != nil {
			discountDeltas[name] = PriceDeltaV2{
				ID:            d.ID,
				TotalDelta:    d.TotalDelta,
				NetPriceDelta: d.NetPriceDelta,
				TopupDelta:    d.TopupDelta,
				SysFeeDelta:   d.SysFeeDelta,
				AgFeeDelta:    d.AgFeeDelta,
			}
		}
	}

	ruleDeltas := make(map[string]PriceDeltaV2)
	for name, d := range o.Price.RuleDelta {
		if d != nil {
			ruleDeltas[name] = PriceDeltaV2{
				ID:            d.ID,
				TotalDelta:    d.TotalDelta,
				NetPriceDelta: d.NetPriceDelta,
				TopupDelta:    d.TopupDelta,
				SysFeeDelta:   d.SysFeeDelta,
				AgFeeDelta:    d.AgFeeDelta,
			}
		}
	}

	return TravelOptionV2{
		TripKey:         o.TripKey,
		IntegrationCode: o.IntegrationCode,
		Price: PriceV2{
			Total:          o.Price.Total,
			FXCode:         o.Price.FXCode,
			PriceLevel:     o.Price.PriceLevel,
			IsValid:        o.Price.IsValid,
			Fares:          fares,
			DiscountDeltas: discountDeltas,
			RuleDeltas:     ruleDeltas,
		},
		AvailableSeats: o.AvailableSeats,
		DepartureTime:  o.DepartureTime,
	}
}
