package response

import (
	"crypto/md5"
	"fmt"
	"strconv"

	"github.com/12go/f4/internal/domain"
)

// SearchResultFullV1 is the API v1 full response, matching PHP SearchResultFullApiV1.
type SearchResultFullV1 struct {
	Trips        []TripV1              `json:"trips"`
	Recheck      []string              `json:"recheck"`
	Stations     map[string]StationV1  `json:"stations"`
	Operators    map[string]OperatorV1 `json:"operators"`
	Classes      map[string]ClassV1    `json:"classes"`
	ProvinceName string                `json:"provinceName,omitempty"`
}

// ClassV1 matches PHP Classs DTO.
type ClassV1 struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	IsMultiPax bool   `json:"is_multi_pax"`
}

// OperatorV1 matches PHP Operator/OperatorExtended DTO.
type OperatorV1 struct {
	ID                 int    `json:"id"`
	MasterID           int    `json:"master_id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Code               any    `json:"code"`                 // string or null
	Logo               any    `json:"logo"`                 // 5-array or null
	ShowSeatsAvailable bool   `json:"show_seats_available"` // always true
	CounterpartID      int    `json:"counterpart_id"`
	Rating             any    `json:"rating"`  // float or null
	Reviews            int    `json:"reviews"` // review count
}

// StationV1 matches PHP Station DTO.
type StationV1 struct {
	StationID           int     `json:"station_id"`
	ProvinceID          int     `json:"province_id"`
	CountryID           string  `json:"country_id"`
	StationName         string  `json:"station_name"`
	StationNameFull     string  `json:"station_name_full"`
	StationCode         any     `json:"station_code"` // string or null
	StationSlug         string  `json:"station_slug"`
	StationLat          float64 `json:"station_lat"`
	StationLng          float64 `json:"station_lng"`
	CoordinatesAccurate bool    `json:"coordinates_accurate"`
	Weight              int     `json:"weight"`
	Map                 any     `json:"map"` // null for now
}

// TripV1 matches PHP TripResultApiV1.
type TripV1 struct {
	ID                     string           `json:"id"`
	ChunkKey               string           `json:"chunk_key"`
	RouteName              string           `json:"route_name"`
	Params                 ParamsV1         `json:"params"`
	Segments               []SegmentV1      `json:"segments"`
	ShowMap                bool             `json:"show_map"`
	Tags                   []any            `json:"tags"`
	TravelOptions          []TravelOptionV1 `json:"travel_options"`
	TransferID             string           `json:"transfer_id"`
	ConnectedWith          any              `json:"connected_with"`
	ScoreSorting           float64          `json:"score_sorting"`
	SalesSorting           float64          `json:"sales_sorting"`
	BookingsLastMonth      int              `json:"bookings_last_month"`
	IsSoloTraveler         bool             `json:"is_solo_traveler"`
	IsBoosted              bool             `json:"is_boosted"`
	OperatorReviewSnippets []any            `json:"operator_review_snippets"`
}

// ParamsV1 matches PHP ParamsApiV1.
type ParamsV1 struct {
	From        int      `json:"from"`
	To          int      `json:"to"`
	DepTime     string   `json:"dep_time"`
	ArrTime     string   `json:"arr_time"`
	Duration    int      `json:"duration"`
	Stops       int      `json:"stops"`
	Vehclasses  []string `json:"vehclasses"`
	Operators   []int    `json:"operators"`
	Bookable    int      `json:"bookable"`
	MinPrice    *PriceV1 `json:"min_price"`
	MinRating   any      `json:"min_rating"`
	RatingCount any      `json:"rating_count"`
	Status      int      `json:"status"`
	IsBookable  int      `json:"is_bookable"`
	Disabled    int      `json:"disabled"`
	Reason      *string  `json:"reason"`
	Hide        bool     `json:"hide"`
	Date        string   `json:"date"`
}

// PriceV1 matches PHP Price DTO — simple {value, fxcode}.
type PriceV1 struct {
	Value  any `json:"value"`  // float64 or null
	FXCode any `json:"fxcode"` // string or null
}

// SegmentV1 matches PHP RouteSegmentApiV1.
type SegmentV1 struct {
	Type                string   `json:"type"`
	TripID              string   `json:"trip_id"`
	OfficialID          any      `json:"official_id"`
	Vehclasses          []string `json:"vehclasses"`
	Price               any      `json:"price"`
	From                int      `json:"from"`
	To                  int      `json:"to"`
	DepTime             string   `json:"dep_time"`
	ArrTime             string   `json:"arr_time"`
	Duration            int      `json:"duration"`
	Class               int      `json:"class"`
	Operator            int      `json:"operator"`
	Rating              any      `json:"rating"`
	Photos              []any    `json:"photos"`
	SearchResultsMarker any      `json:"search_results_marker"`
	ShowMap             bool     `json:"show_map"`
}

// TravelOptionV1 matches PHP TravelOptionApiV1.
type TravelOptionV1 struct {
	ID                  string        `json:"id"`
	Bookable            int           `json:"bookable"`
	Price               *PriceV1      `json:"price"`
	Buy                 []BuyItemV1   `json:"buy"`
	Labels              []any         `json:"labels"`
	Features            any           `json:"features"`
	Class               int           `json:"class"`
	Amenities           []string      `json:"amenities"`
	TicketType          string        `json:"ticket_type"`
	ConfirmationTime    int           `json:"confirmation_time"`
	ConfirmationMinutes int           `json:"confirmation_minutes"`
	ConfirmationMessage string        `json:"confirmation_message"`
	Cancellation        int           `json:"cancellation"`
	FullRefundUntil     *string       `json:"full_refund_until"`
	CancellationMessage string        `json:"cancellation_message"`
	Baggage             any           `json:"baggage"`
	Rating              any           `json:"rating"`
	RatingCount         int           `json:"rating_count"`
	Photos              []any         `json:"photos"`
	IsBookable          int           `json:"is_bookable"`
	Reason              any           `json:"reason"`
	BookingURI          *string       `json:"booking_uri"`
	BookingsLastMonth   int           `json:"bookings_last_month"`
	SalesSorting        float64       `json:"sales_sorting"`
}

// BuyItemV1 matches PHP BuyItem DTO.
type BuyItemV1 struct {
	TripID string  `json:"trip_id"`
	FromID int     `json:"from_id"`
	ToID   int     `json:"to_id"`
	Date   string  `json:"date"`
	Date2  *string `json:"date2"`
	Date3  *string `json:"date3"`
}

// FromDomain converts domain types to API v1 response matching PHP output.
func FromDomain(
	trips []domain.TripResult,
	recheck []string,
	stations map[int]domain.Station,
	operators map[int]domain.Operator,
	classes map[int]domain.VehicleClass,
	provinceName string,
) SearchResultFullV1 {
	v1Trips := make([]TripV1, 0, len(trips))
	for _, t := range trips {
		v1Trips = append(v1Trips, tripToV1(t))
	}

	v1Stations := make(map[string]StationV1, len(stations))
	for id, s := range stations {
		v1Stations[strconv.Itoa(id)] = stationToV1(s)
	}

	v1Operators := make(map[string]OperatorV1, len(operators))
	for id, o := range operators {
		v1Operators[strconv.Itoa(id)] = operatorToV1(o)
	}

	v1Classes := make(map[string]ClassV1, len(classes))
	for id, c := range classes {
		v1Classes[strconv.Itoa(id)] = ClassV1{
			ID:         c.ClassID,
			Name:       c.Name,
			IsMultiPax: c.IsMultiPax,
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

func stationToV1(s domain.Station) StationV1 {
	// PHP preserves the exact DB value: NULL→null, ""→""
	var code any
	if s.StationCode != nil {
		code = *s.StationCode
	}

	nameFull := s.StationNameFull
	if nameFull == "" {
		nameFull = s.StationName
	}

	return StationV1{
		StationID:           s.StationID,
		ProvinceID:          s.ProvinceID,
		CountryID:           s.CountryID,
		StationName:         s.StationName,
		StationNameFull:     nameFull,
		StationCode:         code,
		StationSlug:         s.StationSlug,
		StationLat:          s.Lat,
		StationLng:          s.Lng,
		CoordinatesAccurate: s.CoordinatesAccurate,
		Weight:              s.Weight,
		Map:                 nil,
	}
}

func operatorToV1(o domain.Operator) OperatorV1 {
	masterID := o.MasterID
	if masterID == 0 {
		masterID = o.OperatorID
	}

	var code any
	if o.Code == nil {
		code = nil
	} else {
		code = *o.Code
	}

	var rating any
	if o.RatingAvg > 0 {
		rating = o.RatingAvg
	}

	return OperatorV1{
		ID:                 o.OperatorID,
		MasterID:           masterID,
		Name:               o.Name,
		Slug:               o.Slug,
		Code:               code,
		Logo:               nil, // TODO: load from image table
		ShowSeatsAvailable: true,
		CounterpartID:      o.CounterpartID,
		Rating:             rating,
		Reviews:            o.RatingCount,
	}
}

func tripToV1(t domain.TripResult) TripV1 {
	// Generate MD5 id from group key (matching PHP TripResultBaseFactory::getTripId)
	id := fmt.Sprintf("%x", md5.Sum([]byte(t.GroupKey)))

	segments := make([]SegmentV1, 0, len(t.Segments))
	for _, s := range t.Segments {
		segments = append(segments, segmentToV1(s))
	}

	options := make([]TravelOptionV1, 0, len(t.TravelOptions))
	for _, o := range t.TravelOptions {
		options = append(options, travelOptionToV1(o))
	}

	// Tags: convert string slice to []any for JSON
	tags := make([]any, 0, len(t.Tags))
	for _, tag := range t.Tags {
		tags = append(tags, tag)
	}

	// Build params
	var minPrice *PriceV1
	if t.ParamsMinPrice != nil {
		minPrice = &PriceV1{
			Value:  t.ParamsMinPrice.Value,
			FXCode: t.ParamsMinPrice.FXCode,
		}
	}

	var minRating any
	if t.ParamsMinRating != nil {
		minRating = *t.ParamsMinRating
	}

	var ratingCount any
	if t.ParamsRatingCount != nil {
		ratingCount = *t.ParamsRatingCount
	}

	var reason *string
	if t.ParamsReason != "" {
		r := t.ParamsReason
		reason = &r
	}

	reviewSnippets := t.OperatorReviewSnippets
	if reviewSnippets == nil {
		reviewSnippets = []any{}
	}

	return TripV1{
		ID:        id,
		ChunkKey:  t.ChunkKey,
		RouteName: t.RouteName,
		Params: ParamsV1{
			From:        t.ParamsFrom,
			To:          t.ParamsTo,
			DepTime:     t.ParamsDepTime,
			ArrTime:     t.ParamsArrTime,
			Duration:    t.ParamsDuration,
			Stops:       t.ParamsStops,
			Vehclasses:  t.ParamsVehclasses,
			Operators:   t.ParamsOperators,
			Bookable:    t.ParamsBookable,
			MinPrice:    minPrice,
			MinRating:   minRating,
			RatingCount: ratingCount,
			Status:      t.ParamsStatus,
			IsBookable:  t.ParamsIsBookable,
			Disabled:    t.ParamsDisabled,
			Reason:      reason,
			Hide:        t.ParamsHide,
			Date:        t.ParamsDate,
		},
		Segments:               segments,
		ShowMap:                t.ShowMap,
		Tags:                   tags,
		TravelOptions:          options,
		TransferID:             t.TransferID,
		ConnectedWith:          t.ConnectedWith,
		ScoreSorting:           t.ScoreSorting,
		SalesSorting:           t.SalesSorting,
		BookingsLastMonth:      t.BookingsLastMonth,
		IsSoloTraveler:         t.IsSoloTraveler,
		IsBoosted:              t.IsBoosted,
		OperatorReviewSnippets: reviewSnippets,
	}
}

func segmentToV1(s domain.Segment) SegmentV1 {
	var officialID any
	if s.OfficialID == "" {
		officialID = nil
	} else {
		officialID = s.OfficialID
	}

	vehclasses := s.Vehclasses
	if vehclasses == nil {
		vehclasses = []string{}
	}

	photos := s.Photos
	if photos == nil {
		photos = []any{}
	}

	return SegmentV1{
		Type:                s.Type,
		TripID:              s.TripID,
		OfficialID:          officialID,
		Vehclasses:          vehclasses,
		Price:               s.Price,
		From:                s.FromStationID,
		To:                  s.ToStationID,
		DepTime:             s.Departure,
		ArrTime:             s.Arrival,
		Duration:            s.Duration,
		Class:               s.ClassID,
		Operator:            s.OperatorID,
		Rating:              s.Rating,
		Photos:              photos,
		SearchResultsMarker: s.SearchResultsMarker,
		ShowMap:             s.ShowMap,
	}
}

func travelOptionToV1(o domain.TravelOption) TravelOptionV1 {
	var price *PriceV1
	if o.Price.IsValid {
		price = &PriceV1{
			Value:  o.Price.Total,
			FXCode: o.Price.FXCode,
		}
	}

	amenities := o.Amenities
	if amenities == nil {
		amenities = []string{}
	}

	photos := o.Photos
	if photos == nil {
		photos = []any{}
	}

	labels := o.Labels
	if labels == nil {
		labels = []any{}
	}

	features := o.Features
	if features == nil {
		features = []any{}
	}

	var bookingURI *string
	if o.BookingURL != "" {
		bookingURI = &o.BookingURL
	}

	// Build buy items
	buyItems := make([]BuyItemV1, 0, len(o.Buy))
	for _, b := range o.Buy {
		buyItems = append(buyItems, BuyItemV1{
			TripID: b.TripID,
			FromID: b.FromID,
			ToID:   b.ToID,
			Date:   b.Date,
			Date2:  b.Date2,
			Date3:  b.Date3,
		})
	}
	if buyItems == nil {
		buyItems = []BuyItemV1{}
	}

	return TravelOptionV1{
		ID:                  o.ID,
		Bookable:            o.Bookable,
		Price:               price,
		Buy:                 buyItems,
		Labels:              labels,
		Features:            features,
		Class:               o.ClassID,
		Amenities:           amenities,
		TicketType:          o.TicketType,
		ConfirmationTime:    o.ConfirmationTime,
		ConfirmationMinutes: o.ConfirmMinutes,
		ConfirmationMessage: o.ConfirmMessage,
		Cancellation:        o.Cancellation,
		FullRefundUntil:     o.FullRefundUntil,
		CancellationMessage: o.CancellationMsg,
		Baggage:             o.Baggage,
		Rating:              o.Rating,
		RatingCount:         o.RatingCount,
		Photos:              photos,
		IsBookable:          o.IsBookable,
		Reason:              o.Reason,
		BookingURI:          bookingURI,
		BookingsLastMonth:   o.BookingsLastMonth,
		SalesSorting:        o.SalesSorting,
	}
}
