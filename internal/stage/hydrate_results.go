package stage

import (
	"context"
	"crypto/md5"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

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

func (s *HydrateResultsStage) getTripPhotos(raw domain.RawTrip, in EnrichedTrips) []any {
	if in.Images == nil {
		return []any{}
	}
	photos := in.Images.GetTripImages(
		raw.OperatorID, raw.ClassID,
		raw.OfficialID,
		raw.DepStationID, raw.ArrStationID,
		raw.RouteID,
	)
	if photos == nil {
		return []any{}
	}
	return photos
}

func (s *HydrateResultsStage) hydrateTrip(raw domain.RawTrip, in EnrichedTrips) domain.TripResult {
	// PHP: $travelOption->isBookable = op_bookable && available_seats > 0
	isBookable := raw.OpBookable && raw.Price.Avail > 0

	// PHP Search.php: $showUnavailable = $rawTrip['hide_days'] !== null && $isWebRequest &&
	//   ($rawTrip['hide_days'] == 0 || ($now > $rawTrip['godate'] - (60 * 60 * 24 * $rawTrip['hide_days'])));
	showUnavailable := raw.HideDaysIsSet && !in.Filter.IsBot &&
		(raw.HideDays == 0 || time.Now().Unix() > raw.Godate-int64(raw.HideDays)*86400)

	tr := domain.TripResult{
		TripKey:         raw.TripKey,
		IsBookable:      isBookable,
		HasValidPrice:   raw.Price.IsValid,
		ShowUnavailable: showUnavailable,
		RankScore:       raw.RankScoreFormula,
		SpecialDeal:     raw.SpecialDealFlag,
		NewTrip:         raw.NewTripFlag,
		IsConnection:    raw.SetID != nil || raw.Departure2Time > 0,

		// PHP-matching fields
		ChunkKey:               buildRecheckChunkKey(raw, in.ManualIntegrationID),
		RouteName:              "route name",
		ShowMap:                true,
		ScoreSorting:           calculateRankScoreBySales(raw),
		SalesSorting:           calculateRankSales(float64(raw.Bookings30d)),
		BookingsLastMonth:      bucketSalesPerMonth(raw.SalesPerMonth),
		IsSoloTraveler:         raw.Bookings30d >= 30 && float64(raw.Bookings30dSolo)/float64(raw.Bookings30d)*100 >= 30,
		IsBoosted:              false,
		OperatorReviewSnippets: []any{},
		ConnectedWith:          nil,

		// Params
		ParamsFrom:       raw.DepStationID,
		ParamsTo:         raw.ArrStationID,
		ParamsDepTime:    raw.Dep,
		ParamsArrTime:    trimDateTime(raw.Arr),
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

	// Rating: use trip-level ratings from trip_pool4_extra (tpe.rating_avg, tpe.rating_count).
	// PHP FEATURE_OPERATOR_RATING_ON_TRIP_CARD is off for this comparison.
	if raw.RatingAvg > 0 {
		r := raw.RatingAvg
		tr.ParamsMinRating = &r
		rc := raw.RatingCount
		tr.ParamsRatingCount = &rc
	}

	// Reason: PHP ChiefCook looks up reason text from trip_unavailable_reason table.
	// Only shown for non-bookable trips; bookable trips have no blocking reason.
	if !isBookable && raw.Price.ReasonID > 0 {
		if text, ok := in.ReasonTexts[raw.Price.ReasonID]; ok {
			reason := text
			if raw.Price.ReasonParam > 0 {
				reason = strings.Replace(reason, "[count]", fmt.Sprintf("%d", raw.Price.ReasonParam), 1)
			}
			tr.ParamsReason = reason
		}
	}

	// TransferID = MD5(operatorID;fromID;toID;vehclassID;classID)
	tr.TransferID = md5Hash(fmt.Sprintf("%d;%d;%d;%s;%d",
		raw.OperatorID, raw.DepStationID, raw.ArrStationID, raw.VehclassID, raw.ClassID))

	// Build group key for merging duplicates (matches PHP TripResultBaseFactory::getTripId)
	tr.GroupKey = buildTripUniqueKey(raw)

	// Resolve photos for this trip (same photos used for segment and travel option)
	photos := s.getTripPhotos(raw, in)

	// Build primary segment
	seg := domain.Segment{
		FromStationID:       raw.DepStationID,
		ToStationID:         raw.ArrStationID,
		Departure:           raw.Dep,
		Arrival:             trimDateTime(raw.Arr),
		Duration:            raw.Duration,
		OperatorID:          raw.OperatorID,
		ClassID:             raw.ClassID,
		VehclassID:          raw.VehclassID,
		Type:                "route",
		TripID:              raw.TripKey,
		OfficialID:          raw.OfficialID,
		Vehclasses:          splitVehclass(raw.VehclassID),
		Photos:              photos,
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

	salesSorting := calculateRankSales(float64(raw.Bookings30d))
	if !isBookable {
		salesSorting = 0
	}

	opt := domain.TravelOption{
		Price:           raw.Price,
		TripKey:         raw.TripKey,
		IntegrationCode: raw.IntegrationCode,
		IntegrationID:   effectiveIntegrationID(raw, in.ManualIntegrationID),
		ChunkKey:        buildRecheckChunkKey(raw, in.ManualIntegrationID),
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
		RatingCount:       nilIntIfZero(raw.RatingCount),
		Amenities:         splitAmenities(raw.Amenities),
		TicketType:        defaultStr(raw.TicketType, "default"),
		ConfirmationTime:  confirmDays(raw.ConfirmMinutes),
		ConfirmMinutes:    raw.ConfirmMinutes,
		ConfirmMessage:    buildConfirmMessage(raw.AvgConfirmTime),
		Cancellation:      cancelValue(raw.CancelHours, raw.IsFRefundable),
		CancellationMsg:   cancelMessage(raw.CancelHours, raw.IsFRefundable),
		FullRefundUntil:   buildFullRefundUntil(raw),
		Baggage:           buildBaggage(raw.BaggageFreeWeight),
		Photos:            photos,
		Labels:            []any{},
		Features:          []any{},
		IsBookable:        boolToInt(isBookable),
		Reason:            nil,
		BookingsLastMonth: bucketSalesPerMonth(raw.SalesPerMonth),
		SalesSorting:      salesSorting,
		BookingURL:        buildBookingURI(raw, depGodate),
		Buy:               buildBuyItems(raw, depGodate),
		FromStationID:     raw.DepStationID,
		ToStationID:       raw.ArrStationID,
		DepGodate:         depGodate,
		DepGodate2:        depGodate2,
		DepGodate3:        depGodate3,
		PriceRestriction:  raw.PriceRestriction,
		Bookings30d:       raw.Bookings30d,
		Bookings30dSolo:   raw.Bookings30dSolo,
		ScoreSortingRaw:   calculateRankScoreBySales(raw),
	}

	// Price breakdown from adult fare (conditional, matching PHP TravelOptionBaseFactory)
	if adultFare := raw.Price.Fares["adult"]; adultFare != nil {
		if in.Filter.NeedPassTopup {
			opt.AgFee = &domain.PriceSimple{Value: adultFare.AgFee, FXCode: adultFare.AgFeeFXCode}
		}
		if in.Filter.NeedPassNetpriceAndSysfee {
			opt.NetPriceDetail = &domain.PriceSimple{Value: adultFare.NetPrice, FXCode: adultFare.NetPriceFXCode}
			opt.SysFeeDetail = &domain.PriceSimple{Value: adultFare.SysFee, FXCode: adultFare.SysFeeFXCode}
		}
	}
	tr.TravelOptions = []domain.TravelOption{opt}

	// PHP builds tags from segment-level route features (RouteSegmentApiV1::tags),
	// not from trip-level amenities/ticket_type. Leave empty for now.
	tr.Tags = []string{}

	return tr
}

func (s *HydrateResultsStage) buildTags(raw domain.RawTrip) []string {
	var tags []string

	if raw.Amenities != "" {
		for _, a := range strings.Split(raw.Amenities, ",") {
			a = strings.ToLower(strings.TrimSpace(a))
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
		p = strings.ToLower(strings.TrimSpace(p))
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

// trimDateTime strips any sub-second suffix (e.g. ".000000") from MySQL DATETIME strings.
func trimDateTime(s string) string {
	if len(s) > 19 {
		return s[:19]
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
		return "No refunds, no cancellation"
	}
	if refundable {
		return fmt.Sprintf("Full refund up to %d hours before departure", hours)
	}
	return "No refunds, no cancellation"
}

// buildConfirmMessage formats the confirmation message from avg_confirm_time.
// avg_confirm_time is stored in SECONDS in the DB (matching PHP AvgConfirmationTime service).
func buildConfirmMessage(avgConfirmTimeSecs int) string {
	if avgConfirmTimeSecs <= 0 {
		return "Usually confirmed within avg_confirm_time_after_paid"
	}
	minutes := int(math.Ceil(float64(avgConfirmTimeSecs) / 60.0))
	if minutes <= 5 {
		return "Instant confirmation"
	}
	hours := int(math.Ceil(float64(avgConfirmTimeSecs) / 3600.0))
	if hours < 24 {
		if hours == 1 {
			return "Usually confirmed within 1 hour after paid"
		}
		return fmt.Sprintf("Usually confirmed within %d hours after paid", hours)
	}
	days := int(math.Ceil(float64(hours) / 24.0))
	if days == 1 {
		return "Usually confirmed within 1 day after paid"
	}
	return fmt.Sprintf("Usually confirmed within %d days after paid", days)
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

// calculateRankScoreBySales matches PHP TravelOptionBaseFactory::calculateRankScoreBySales().
// Uses default parameters: rankScoreSalesMultiplierPct=100, newTripPenaltyFactor=0.01,
// specialDealScoreFallback=0.1, specialDealMultiplier=10.0.
func calculateRankScoreBySales(raw domain.RawTrip) float64 {
	k := 1.0
	if raw.NewTripFlag {
		k = 0.01
	}
	score := raw.RankScoreSales * k * 100 // rankScoreSalesMultiplierPct=100
	// rankScoreSysfee * (100-100) = 0, so sysfee term is zero with default config
	if raw.SpecialDealFlag {
		if score < 0.1 {
			score = 0.1
		}
		return score * 10.0 // specialDealMultiplier / specialDealFlag(=1 when true)
	}
	return score
}

// calculateRankSales matches PHP TravelOptionBaseFactory::calculateRankSales().
// RANK_SCORE_SALES_REAL_MULTIPLIER = 40.
func calculateRankSales(bookings30d float64) float64 {
	if bookings30d >= 1.0 {
		return math.Log(bookings30d * 40)
	}
	return 0
}

// bucketSalesPerMonth matches PHP TravelOptionBaseFactory::convertTripRankScoreSalesReal().
// It buckets sales_per_month into display-friendly tiers for bookings_last_month.
func bucketSalesPerMonth(v int) int {
	if v < 15 {
		return 0
	}
	if v < 50 {
		return 15
	}
	if v < 100 {
		return 50
	}
	if v < 500 {
		return 100
	}
	if v < 1000 {
		return 500
	}
	return 1000
}

func nilIntIfZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// effectiveIntegrationID returns the corrected integration ID for a raw trip,
// matching PHP's logic: when integrationCode is "manual" (no integration row)
// and a manualIntegrationID is configured, use that instead of 0.
func effectiveIntegrationID(raw domain.RawTrip, manualIntegrationID int) int {
	if raw.IntegrationCode == "manual" && raw.IntegrationID == 0 && manualIntegrationID > 0 {
		return manualIntegrationID
	}
	return raw.IntegrationID
}

// buildRecheckChunkKey computes the chunk_key (group key) matching PHP TripResultBaseFactory::getRecheckChunkKey().
// PHP: integrationId + chunk_key field values joined by "-".
func buildRecheckChunkKey(raw domain.RawTrip, manualIntegrationID int) string {
	integrationID := effectiveIntegrationID(raw, manualIntegrationID)
	chunkKey := raw.ChunkKey

	// PHP line 38: if ($integrationCode === 'manual') { $chunkKey = 'date'; }
	// Unconditional override for all manual-integration trips (both LEFT JOIN matched and NULL fallback).
	if raw.IntegrationCode == "manual" {
		chunkKey = "date"
	} else if raw.VehclassID == "train" && strings.Contains(raw.IntegrationCode, "easybook") {
		chunkKey = "vehclass_id,dep_station_id,arr_station_id"
	}

	fields := strings.Split(chunkKey, ",")
	values := []string{strconv.Itoa(integrationID)}

	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if field == "date" {
			// PHP uses date('Y-m-d', $rawTrip['godate']) with default timezone Asia/Bangkok
			// (set in docker/php.ini: date.timezone = Asia/Bangkok).
			// We must match that: interpret the godate Unix timestamp in Bangkok timezone.
			loc, _ := time.LoadLocation("Asia/Bangkok")
			t := time.Unix(raw.Godate, 0).In(loc)
			values = append(values, t.Format("2006-01-02"))
			continue
		}
		values = append(values, getRawTripField(raw, field))
	}

	return strings.Join(values, "-")
}

// getRawTripField returns a raw trip field value by name, matching PHP $rawTrip[$field].
func getRawTripField(raw domain.RawTrip, field string) string {
	switch field {
	case "vehclass_id":
		return raw.VehclassID
	case "dep_station_id":
		return strconv.Itoa(raw.DepStationID)
	case "arr_station_id":
		return strconv.Itoa(raw.ArrStationID)
	case "operator_id":
		return strconv.Itoa(raw.OperatorID)
	case "class_id":
		return strconv.Itoa(raw.ClassID)
	case "official_id":
		return raw.OfficialID
	default:
		return "?"
	}
}

// buildFullRefundUntil calculates the full refund deadline, matching PHP
// TravelOptionBaseFactory::buildFullRefundUntil().
// Returns "YYYY-MM-DD HH:MM" or nil.
func buildFullRefundUntil(raw domain.RawTrip) *string {
	if !raw.IsFRefundable || raw.CancelHours == 0 {
		return nil
	}
	loc, err := time.LoadLocation(raw.DepTimezoneName)
	if err != nil {
		return nil
	}
	depTime, err := time.ParseInLocation("2006-01-02 15:04:05", raw.Dep, loc)
	if err != nil {
		return nil
	}
	now := time.Now().In(loc)
	if !now.Before(depTime) {
		return nil
	}
	result := depTime.Add(-time.Duration(raw.CancelHours) * time.Hour)
	if result.Before(now) {
		return nil
	}
	s := result.Format("2006-01-02 15:04")
	return &s
}
