package response

import "github.com/12go/f4/internal/domain"

// SearchResultFullV1 is the API v1 full response, matching PHP SearchResultFullApiV1.
type SearchResultFullV1 struct {
	Trips        []TripResultV1            `json:"trips"`
	Recheck      []string                  `json:"recheck"`
	Stations     map[int]StationV1         `json:"stations,omitempty"`
	Operators    map[int]OperatorV1        `json:"operators,omitempty"`
	Classes      map[int]ClassV1           `json:"classes,omitempty"`
	ProvinceName string                    `json:"provinceName,omitempty"`
	Admin        map[string]any            `json:"admin,omitempty"`
}

// TripResultV1 is the API v1 trip representation.
type TripResultV1 struct {
	TripKey       string            `json:"trip_key"`
	GroupKey      string            `json:"group_key"`
	Segments      []SegmentV1       `json:"segments"`
	TravelOptions []TravelOptionV1  `json:"travel_options"`
	Tags          []string          `json:"tags,omitempty"`
	IsBookable    bool              `json:"is_bookable"`
	HasValidPrice bool              `json:"has_valid_price"`
	IsConnection  bool              `json:"is_connection"`
	RankScore     float64           `json:"rank_score"`
	SpecialDeal   bool              `json:"special_deal,omitempty"`
	NewTrip       bool              `json:"new_trip,omitempty"`
}

// SegmentV1 represents a single leg of a trip in API v1.
type SegmentV1 struct {
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

// TravelOptionV1 represents a bookable option in API v1.
type TravelOptionV1 struct {
	TripKey         string    `json:"trip_key"`
	IntegrationCode string   `json:"integration_code"`
	Price           PriceV1  `json:"price"`
	AvailableSeats  int      `json:"available_seats"`
	DepartureTime   int      `json:"departure_time"`
}

// PriceV1 is the API v1 price representation.
type PriceV1 struct {
	Total      float64           `json:"total"`
	FXCode     string            `json:"fxcode"`
	PriceLevel int               `json:"price_level"`
	IsValid    bool              `json:"is_valid"`
	Fares      map[string]FareV1 `json:"fares,omitempty"`
}

// FareV1 is a single fare block in API v1.
type FareV1 struct {
	FullPrice float64 `json:"full_price"`
	FXCode    string  `json:"fxcode"`
	NetPrice  float64 `json:"net_price,omitempty"`
	Topup     float64 `json:"topup,omitempty"`
	SysFee    float64 `json:"sys_fee,omitempty"`
}

// StationV1 is the station dictionary entry in API v1.
type StationV1 struct {
	StationID   int     `json:"station_id"`
	StationName string  `json:"station_name"`
	StationCode string  `json:"station_code,omitempty"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	ProvinceID  int     `json:"province_id"`
	CountryID   string  `json:"country_id"`
	VehclassID  string  `json:"vehclass_id"`
}

// OperatorV1 is the operator dictionary entry in API v1.
type OperatorV1 struct {
	OperatorID  int     `json:"operator_id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	LogoURL     string  `json:"logo_url,omitempty"`
	RatingAvg   float64 `json:"rating_avg,omitempty"`
	RatingCount int     `json:"rating_count,omitempty"`
}

// ClassV1 is the class dictionary entry in API v1.
type ClassV1 struct {
	ClassID    int    `json:"class_id"`
	Name       string `json:"name"`
	Vehclasses string `json:"vehclasses"`
}

// FromDomain converts domain types to API v1 response.
func FromDomain(trips []domain.TripResult, recheck []string, stations map[int]domain.Station, operators map[int]domain.Operator, classes map[int]domain.VehicleClass, provinceName string) SearchResultFullV1 {
	v1Trips := make([]TripResultV1, 0, len(trips))
	for _, t := range trips {
		v1Trips = append(v1Trips, tripToV1(t))
	}

	v1Stations := make(map[int]StationV1, len(stations))
	for id, s := range stations {
		v1Stations[id] = StationV1{
			StationID:   s.StationID,
			StationName: s.StationName,
			StationCode: s.StationCode,
			Lat:         s.Lat,
			Lng:         s.Lng,
			ProvinceID:  s.ProvinceID,
			CountryID:   s.CountryID,
			VehclassID:  s.VehclassID,
		}
	}

	v1Operators := make(map[int]OperatorV1, len(operators))
	for id, o := range operators {
		v1Operators[id] = OperatorV1{
			OperatorID:  o.OperatorID,
			Name:        o.Name,
			Slug:        o.Slug,
			LogoURL:     o.LogoURL,
			RatingAvg:   o.RatingAvg,
			RatingCount: o.RatingCount,
		}
	}

	v1Classes := make(map[int]ClassV1, len(classes))
	for id, c := range classes {
		v1Classes[id] = ClassV1{
			ClassID:    c.ClassID,
			Name:       c.Name,
			Vehclasses: c.Vehclasses,
		}
	}

	if recheck == nil {
		recheck = []string{}
	}

	return SearchResultFullV1{
		Trips:        v1Trips,
		Recheck:      recheck,
		Stations:     v1Stations,
		Operators:    v1Operators,
		Classes:      v1Classes,
		ProvinceName: provinceName,
	}
}

func tripToV1(t domain.TripResult) TripResultV1 {
	segments := make([]SegmentV1, 0, len(t.Segments))
	for _, s := range t.Segments {
		segments = append(segments, SegmentV1{
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

	options := make([]TravelOptionV1, 0, len(t.TravelOptions))
	for _, o := range t.TravelOptions {
		options = append(options, travelOptionToV1(o))
	}

	return TripResultV1{
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

func travelOptionToV1(o domain.TravelOption) TravelOptionV1 {
	fares := make(map[string]FareV1)
	for name, f := range o.Price.Fares {
		if f != nil {
			fares[name] = FareV1{
				FullPrice: f.FullPrice,
				FXCode:    f.FullPriceFXCode,
				NetPrice:  f.NetPrice,
				Topup:     f.Topup,
				SysFee:    f.SysFee,
			}
		}
	}

	return TravelOptionV1{
		TripKey:         o.TripKey,
		IntegrationCode: o.IntegrationCode,
		Price: PriceV1{
			Total:      o.Price.Total,
			FXCode:     o.Price.FXCode,
			PriceLevel: o.Price.PriceLevel,
			IsValid:    o.Price.IsValid,
			Fares:      fares,
		},
		AvailableSeats: o.AvailableSeats,
		DepartureTime:  o.DepartureTime,
	}
}
