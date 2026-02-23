package domain

import "time"

// RawTrip represents a single row from the main search SQL query.
type RawTrip struct {
	TripKey        string `db:"trip_key"`
	Duration       int    `db:"duration"` // minutes
	DepartureTime  int    `db:"departure_time"` // minutes from midnight
	Departure2Time int    `db:"departure2_time"`
	Departure3Time int    `db:"departure3_time"`
	ClassID        int    `db:"class_id"`
	OfficialID     string `db:"official_id"`
	OperatorID     int    `db:"operator_id"`
	VehclassID     string `db:"vehclass_id"`
	DepStationID   int    `db:"dep_station_id"`
	ArrStationID   int    `db:"arr_station_id"`
	SetID          *int   `db:"set_id"` // nil if not a connection

	// Station data
	DepTimezoneName string `db:"dep_timezone_name"`
	ArrTimezoneName string `db:"arr_timezone_name"`
	DepCountryID    string `db:"dep_country_id"`
	ArrCountryID    string `db:"arr_country_id"`
	DepProvinceID   int    `db:"dep_province_id"`
	ArrProvinceID   int    `db:"arr_province_id"`
	DepHideDeparture bool  `db:"dep_hide_departure"`

	// Operator data
	OpBookable       bool   `db:"op_bookable"`
	SellerID         int    `db:"seller_id"`
	MasterOperatorID int    `db:"master_operator_id"`
	PriceRestriction int    `db:"price_restriction"`

	// Integration
	IntegrationCode string `db:"integration_code"`
	IntegrationID   int    `db:"integration_id"`
	ChunkKey        string `db:"chunk_key"`

	// Class
	Vehclasses string `db:"vehclasses"`

	// Trip extras
	HideDays          int     `db:"hide_days"`
	AdvanceBook       int     `db:"advance_book"`
	CancelHours       int     `db:"cancel_hours"`
	ConfirmMinutes    int     `db:"confirm_minutes"`
	RatingAvg         float64 `db:"rating_avg"`
	RatingCount       int     `db:"rating_count"`
	SalesPerMonth     int     `db:"sales_per_month"`
	BaggageFreeWeight int     `db:"baggage_free_weight"`
	BaggageFreeHand   int     `db:"baggage_free_hand"`
	BaggageFreeChecked int    `db:"baggage_free_checked"`
	Amenities         string  `db:"amenities"`
	TicketType        string  `db:"ticket_type"`
	SRMarker          string  `db:"sr_marker"`
	IsMeta            bool    `db:"is_meta"`
	IsIgnoreGroupTime bool    `db:"is_ignore_group_time"`
	IsFRefundable     bool    `db:"is_f_refundable"`
	TripID            int     `db:"trip_id"`
	RouteID           int     `db:"route_id"`
	AvgConfirmTime    int     `db:"avg_confirm_time"`

	// Departure extras
	NewTripFlag           bool    `db:"new_trip_flag"`
	SpecialDealFlag       bool    `db:"special_deal_flag"`
	RankScoreSales        float64 `db:"rank_score_sales"`
	RankScoreFormula      float64 `db:"rank_score_formula"`
	RankScoreFormulaRev   float64 `db:"rank_score_formula_revenue"`
	RankScoreSalesReal90  float64 `db:"rank_score_sales_real_90_days"`
	Bookings30d           int     `db:"bookings_30d"`
	Bookings30dSolo       int     `db:"bookings_30d_solo"`

	// Price (decoded from binary)
	PriceBinStr []byte    `db:"price_bin_str"`
	Price       TripPrice `db:"-"`

	// Timestamps
	Godate      int64  `db:"godate"` // unix timestamp
	GodateStamp int64  `db:"godate_stamp"`
	Dep         string `db:"dep"` // formatted departure datetime
	Arr         string `db:"arr"` // formatted arrival datetime

	// Round trip
	RoundTripDiscountPct float64 `db:"round_trip_discount_pct"`
}

// TripPlain is a simplified trip reference used for round trip lookups.
type TripPlain struct {
	TripKey             string
	DepStationID        int
	ArrStationID        int
	OperatorID          int
	IntegrationCode     string
	Godate              time.Time
	DepartureTime       int
	DepTimezoneName     string
	ArrTimezoneName     string
	HasRoundTripDiscount bool
	Price               TripPrice
}

// TripResult is a fully hydrated trip ready for API response.
type TripResult struct {
	TripKey       string
	GroupKey      string // for merging duplicates
	Segments      []Segment
	TravelOptions []TravelOption
	Tags          []string
	IsBookable    bool
	HasValidPrice bool
	IsConnection  bool
	RankScore     float64
	SpecialDeal   bool
	NewTrip       bool
}

// Segment represents one leg of a trip.
type Segment struct {
	FromStationID int
	ToStationID   int
	Departure     string
	Arrival       string
	Duration      int // minutes
	OperatorID    int
	ClassID       int
	VehclassID    string
	Type          string // "route" or "wait"
}

// TravelOption represents a bookable option for a trip.
type TravelOption struct {
	Price           TripPrice
	AvailableSeats  int
	BookingURL      string
	UniqueKey       string // for dedup
	TripKey         string
	IntegrationCode string
	DepartureTime   int
	Departure2Time  int
	Departure3Time  int
}
