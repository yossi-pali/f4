package stage

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"

	"github.com/12go/f4/internal/domain"
)

// HydratedResults is the output of Stage 7.
type HydratedResults struct {
	Trips     []domain.TripResult
	Operators map[int]domain.Operator
	Stations  map[int]domain.Station
	Classes   map[int]domain.VehicleClass
	Filter    domain.SearchFilter
}

// HydrateResultsStage builds TripResult DTOs from raw trips and reference data.
type HydrateResultsStage struct{}

func NewHydrateResultsStage() *HydrateResultsStage { return &HydrateResultsStage{} }

func (s *HydrateResultsStage) Name() string { return "hydrate_results" }

func (s *HydrateResultsStage) Execute(_ context.Context, in EnrichedTrips) (HydratedResults, error) {
	out := HydratedResults{
		Operators: in.Operators,
		Stations:  in.Stations,
		Classes:   in.Classes,
		Filter:    in.Filter,
	}

	results := make([]domain.TripResult, 0, len(in.Trips))
	for _, raw := range in.Trips {
		tr := s.hydrateTrip(raw, in)
		results = append(results, tr)
	}
	out.Trips = results

	return out, nil
}

func (s *HydrateResultsStage) hydrateTrip(raw domain.RawTrip, in EnrichedTrips) domain.TripResult {
	isBookable := raw.OpBookable && raw.Price.IsValid

	tr := domain.TripResult{
		TripKey:       raw.TripKey,
		IsBookable:    isBookable,
		HasValidPrice: raw.Price.IsValid,
		RankScore:     raw.RankScoreFormula,
		SpecialDeal:   raw.SpecialDealFlag,
		NewTrip:       raw.NewTripFlag,
		IsConnection:  raw.SetID != nil || raw.Departure2Time > 0,

		// PHP-matching fields
		ChunkKey:               raw.ChunkKey,
		RouteName:              "route name",
		ShowMap:                true,
		ScoreSorting:           raw.RankScoreFormula,
		SalesSorting:           raw.RankScoreSales,
		BookingsLastMonth:      raw.Bookings30d,
		IsSoloTraveler:         raw.Bookings30d > 0 && raw.Bookings30dSolo == raw.Bookings30d,
		IsBoosted:              false,
		OperatorReviewSnippets: []any{},
		ConnectedWith:          nil,

		// Params
		ParamsFrom:       raw.DepStationID,
		ParamsTo:         raw.ArrStationID,
		ParamsDepTime:    raw.Dep,
		ParamsArrTime:    raw.Arr,
		ParamsDuration:   raw.Duration,
		ParamsStops:      0,
		ParamsVehclasses: splitVehclass(raw.VehclassID),
		ParamsOperators:  []int{raw.OperatorID},
		ParamsBookable:   raw.Price.Avail,
		ParamsStatus:     0,
		ParamsIsBookable: boolToInt(isBookable),
		ParamsDisabled:   0,
		ParamsHide:       false,
		ParamsDate:       "",
	}

	// MinPrice
	if raw.Price.IsValid {
		tr.ParamsMinPrice = &domain.PriceSimple{Value: raw.Price.Total, FXCode: raw.Price.FXCode}
	}

	// Rating
	if raw.RatingAvg > 0 {
		r := raw.RatingAvg
		tr.ParamsMinRating = &r
		rc := raw.RatingCount
		tr.ParamsRatingCount = &rc
	}

	// Reason
	tr.ParamsReason = buildPriceReason(raw)

	// TransferID = MD5(operatorID;fromID;toID;vehclassID;classID)
	tr.TransferID = md5Hash(fmt.Sprintf("%d;%d;%d;%s;%d",
		raw.OperatorID, raw.DepStationID, raw.ArrStationID, raw.VehclassID, raw.ClassID))

	// Build group key for merging duplicates (matches PHP TripResultBaseFactory::getTripId)
	tr.GroupKey = buildTripUniqueKey(raw)

	// Build primary segment
	seg := domain.Segment{
		FromStationID:       raw.DepStationID,
		ToStationID:         raw.ArrStationID,
		Departure:           raw.Dep,
		Arrival:             raw.Arr,
		Duration:            raw.Duration,
		OperatorID:          raw.OperatorID,
		ClassID:             raw.ClassID,
		VehclassID:          raw.VehclassID,
		Type:                "route",
		TripID:              raw.TripKey,
		OfficialID:          raw.OfficialID,
		Vehclasses:          splitVehclass(raw.VehclassID),
		Photos:              []any{},
		Price:               nil,
		Rating:              nil,
		SearchResultsMarker: nil,
		ShowMap:             true,
	}
	tr.Segments = []domain.Segment{seg}

	// Build travel option
	depGodate := formatGodate(raw.Dep)
	var depGodate2, depGodate3 *string
	// Departure2/3 are minutes-from-midnight; they don't produce godate2/3 for single-leg trips
	// For connections, these would be populated differently

	salesSorting := raw.RankScoreSales
	if !isBookable {
		salesSorting = 0
	}

	opt := domain.TravelOption{
		Price:           raw.Price,
		TripKey:         raw.TripKey,
		IntegrationCode: raw.IntegrationCode,
		DepartureTime:   raw.DepartureTime,
		Departure2Time:  raw.Departure2Time,
		Departure3Time:  raw.Departure3Time,
		AvailableSeats:  raw.Price.Avail,
		UniqueKey: fmt.Sprintf("%s-%d-%d-%d",
			raw.TripKey, raw.DepartureTime, raw.Departure2Time, raw.Departure3Time),

		// PHP-matching fields
		ID:                raw.TripKey,
		ClassID:           raw.ClassID,
		Bookable:          raw.Price.Avail,
		Rating:            nilIfZero(raw.RatingAvg),
		RatingCount:       raw.RatingCount,
		Amenities:         splitAmenities(raw.Amenities),
		TicketType:        defaultStr(raw.TicketType, "default"),
		ConfirmationTime:  confirmDays(raw.ConfirmMinutes),
		ConfirmMinutes:    raw.ConfirmMinutes,
		ConfirmMessage:    buildConfirmMessage(raw.AvgConfirmTime),
		Cancellation:      cancelValue(raw.CancelHours, raw.IsFRefundable),
		CancellationMsg:   cancelMessage(raw.CancelHours, raw.IsFRefundable),
		FullRefundUntil:   nil,
		Baggage:           buildBaggage(raw.BaggageFreeWeight),
		Photos:            []any{},
		Labels:            []any{},
		Features:          []any{},
		IsBookable:        boolToInt(isBookable),
		Reason:            nil,
		BookingsLastMonth: raw.Bookings30d,
		SalesSorting:      salesSorting,
		BookingURL:        buildBookingURI(raw, depGodate),
		Buy:               buildBuyItems(raw, depGodate),
		FromStationID:     raw.DepStationID,
		ToStationID:       raw.ArrStationID,
		DepGodate:         depGodate,
		DepGodate2:        depGodate2,
		DepGodate3:        depGodate3,
	}
	tr.TravelOptions = []domain.TravelOption{opt}

	// Build tags
	tr.Tags = s.buildTags(raw)

	return tr
}

func (s *HydrateResultsStage) buildTags(raw domain.RawTrip) []string {
	var tags []string

	if raw.Amenities != "" {
		for _, a := range strings.Split(raw.Amenities, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				tags = append(tags, a)
			}
		}
	}
	if raw.TicketType != "" {
		tags = append(tags, "ticket:"+raw.TicketType)
	}
	if raw.BaggageFreeWeight > 0 {
		tags = append(tags, fmt.Sprintf("baggage:%dkg", raw.BaggageFreeWeight))
	}
	if raw.IsFRefundable {
		tags = append(tags, "refundable")
	}
	if raw.SpecialDealFlag {
		tags = append(tags, "special_deal")
	}
	if raw.NewTripFlag {
		tags = append(tags, "new")
	}

	return tags
}

// --- Helper functions ---

// buildTripUniqueKey builds the trip grouping key matching PHP TripResultBaseFactory::getTripId().
// Format: "{official_id}#{operator_id}#{vehclass_id}#{dep_station_id}#{arr_station_id}#{depTime}#{setIdOrPackKey}"
// For non-grouped single-leg trips, depTime = "YYYY-MM-DD 00:00:00" + class_id.
func buildTripUniqueKey(raw domain.RawTrip) string {
	depTime := raw.Dep // "YYYY-MM-DD HH:MM:SS"

	// For non-connection trips without is_ignore_group_time, zero out the time and append class_id
	isConnection := raw.SetID != nil // approximation: connections have set_id
	if !raw.IsIgnoreGroupTime && !isConnection {
		if len(depTime) >= 10 {
			depTime = depTime[:10] + " 00:00:00" + fmt.Sprintf("%d", raw.ClassID)
		}
	}

	setIDStr := ""
	if raw.SetID != nil {
		setIDStr = fmt.Sprintf("%d", *raw.SetID)
	}

	return fmt.Sprintf("%s#%d#%s#%d#%d#%s#%s",
		raw.OfficialID, raw.OperatorID, raw.VehclassID,
		raw.DepStationID, raw.ArrStationID, depTime, setIDStr)
}

func md5Hash(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}

func splitVehclass(id string) []string {
	if id == "" {
		return []string{}
	}
	return []string{id}
}

func splitAmenities(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nilIfZero(f float64) *float64 {
	if f == 0 {
		return nil
	}
	return &f
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// formatGodate converts "2026-02-06 18:57:00" → "2026-02-06-18-57-00"
func formatGodate(dep string) string {
	r := strings.NewReplacer(" ", "-", ":", "-")
	return r.Replace(dep)
}

func buildBaggage(weight int) map[string]any {
	v := weight
	if v == 0 {
		v = -1
	}
	return map[string]any{
		"value":   v,
		"icon":    nil,
		"message": nil,
	}
}

func buildBookingURI(raw domain.RawTrip, godate string) string {
	return fmt.Sprintf("/en/add-to-cart/%s/%s?seats=1", raw.TripKey, godate)
}

func buildBuyItems(raw domain.RawTrip, godate string) []domain.BuyItemV1 {
	return []domain.BuyItemV1{
		{
			TripID: raw.TripKey,
			FromID: raw.DepStationID,
			ToID:   raw.ArrStationID,
			Date:   godate,
			Date2:  nil,
			Date3:  nil,
		},
	}
}

func confirmDays(minutes int) int {
	if minutes == 0 {
		return 1 // instant confirmation
	}
	days := minutes / 1440
	if days < 1 {
		return 1
	}
	return days
}

func cancelValue(hours int, refundable bool) int {
	if refundable {
		return hours
	}
	return 0
}

func cancelMessage(hours int, refundable bool) string {
	if !refundable && hours == 0 {
		return "No refunds, no cancelation"
	}
	if refundable {
		return fmt.Sprintf("Full refund up to %d hours before departure", hours)
	}
	return "No refunds, no cancelation"
}

func buildConfirmMessage(avgConfirmTime int) string {
	_ = avgConfirmTime
	return "Usually confirmed within avg_confirm_time_after_paid"
}

func buildPriceReason(raw domain.RawTrip) string {
	if raw.Price.ReasonID == 0 {
		return ""
	}
	// Build a short code similar to PHP's price reason encoding
	return fmt.Sprintf("cc%c%d%c",
		reasonLetter(raw.Price.PriceLevel),
		raw.Price.ReasonID,
		reasonLetter2(raw.Price.ReasonParam))
}

func reasonLetter(level int) byte {
	switch level {
	case 0:
		return 'N'
	case 1:
		return 'S'
	case 2:
		return 'A'
	case 3:
		return 'P'
	default:
		return 'U'
	}
}

func reasonLetter2(param int) byte {
	if param == 0 {
		return 'y'
	}
	return 'n'
}
