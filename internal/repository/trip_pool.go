package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/12go/f4/internal/db"
	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/price"
)

// TripPoolRepo handles the main trip search query against regional databases.
type TripPoolRepo struct {
	connMgr  *db.ConnectionManager
	regionR  db.RegionResolver
}

func NewTripPoolRepo(connMgr *db.ConnectionManager, regionR db.RegionResolver) *TripPoolRepo {
	return &TripPoolRepo{connMgr: connMgr, regionR: regionR}
}

// SearchParams holds the parameters for the main search query.
type SearchParams struct {
	FromStationIDs     []int
	ToStationIDs       []int
	FromPlaceID        string // e.g. "1p" — when set and ends with 'p', uses route_place join (PHP behavior)
	ToPlaceID          string // e.g. "44p"
	GodateString       string // "YYYY-MM-DD"
	SeatsAdult         int
	SeatsChild         int
	SeatsInfant        int
	AgentID            int
	Lang               string
	FXCode             string
	RecheckLevel       int
	PriceMode          int
	OperatorIDs        []int
	SellerIDs          []int
	VehclassIDs        []string
	ClassIDs           []int
	CountryIDs         []string
	ExcludeOperatorIDs []int
	ExcludeSellerIDs   []int
	ExcludeVehclassIDs []string
	ExcludeClassIDs    []int
	ExcludeCountryIDs  []string
	IntegrationCode    string
	TripKeys           []string
	OnlyDirect         bool
	Limit              int
}

// Search executes the main trip search query.
// This is the most performance-critical query in the system.
// Ported from PHP TripPoolRepository::search().
func (r *TripPoolRepo) Search(ctx context.Context, p SearchParams) ([]domain.RawTrip, error) {
	if len(p.FromStationIDs) == 0 || len(p.ToStationIDs) == 0 {
		return nil, nil
	}

	region := r.regionR.ResolveByStationID(p.FromStationIDs[0])
	conn := r.connMgr.TripPool(region)

	query, args := r.buildSearchQuery(p)

	var rawRows []rawTripRow
	err := db.WithRetry(func() error {
		return conn.SelectContext(ctx, &rawRows, query, args...)
	})
	if err != nil {
		return nil, fmt.Errorf("trip pool search: %w", err)
	}

	trips := make([]domain.RawTrip, 0, len(rawRows))
	for _, row := range rawRows {
		trip := row.toDomainTrip()
		// Decode binary price
		if len(row.PriceBinStr) > 0 {
			tp, err := price.Decode(row.PriceBinStr)
			if err == nil {
				trip.Price = tp
			}
		}
		trips = append(trips, trip)
	}

	return trips, nil
}

// rawTripRow is the database scan target for the main search query.
type rawTripRow struct {
	TripKey        string `db:"trip_key"`
	Duration       int    `db:"duration"`
	DepartureTime  int    `db:"departure_time"`
	Departure2Time int    `db:"departure2_time"`
	Departure3Time int    `db:"departure3_time"`
	ClassID        int    `db:"class_id"`
	OfficialID     string `db:"official_id"`
	OperatorID     int    `db:"operator_id"`
	VehclassID     string `db:"vehclass_id"`
	DepStationID   int    `db:"dep_station_id"`
	ArrStationID   int    `db:"arr_station_id"`
	SetID          *int   `db:"set_id"`

	DepTimezoneName  string `db:"dep_timezone_name"`
	ArrTimezoneName  string `db:"arr_timezone_name"`
	DepCountryID     string `db:"dep_country_id"`
	ArrCountryID     string `db:"arr_country_id"`
	DepProvinceID    int    `db:"dep_province_id"`
	ArrProvinceID    int    `db:"arr_province_id"`
	DepHideDeparture int `db:"dep_hide_departure"`

	OpBookable       int `db:"op_bookable"`
	SellerID         int    `db:"seller_id"`
	MasterOperatorID int    `db:"master_operator_id"`
	PriceRestriction int    `db:"price_restriction"`

	IntegrationCode string `db:"integration_code"`
	IntegrationID   int    `db:"integration_id"`
	ChunkKey        string `db:"chunk_key"`

	Vehclasses string `db:"vehclasses"`

	// Extras from trip_pool4_extra
	RatingAvg          float64 `db:"rating_avg"`
	RatingCount        int     `db:"rating_count"`
	SalesPerMonth      int     `db:"sales_per_month"`
	BaggageFreeWeight  int     `db:"baggage_free_weight"`
	BaggageFreeHand    int     `db:"baggage_free_hand"`
	BaggageFreeChecked int     `db:"baggage_free_checked"`
	Amenities          string  `db:"amenities"`
	TicketType         string  `db:"ticket_type"`
	SRMarker           string  `db:"sr_marker"`
	IsMeta             int `db:"is_meta"`
	IsIgnoreGroupTime  int `db:"is_ignore_group_time"`
	IsFRefundable      int `db:"is_f_refundable"`
	TripID             int     `db:"trip_id"`
	RouteID            int     `db:"route_id"`
	HideDays           int     `db:"hide_days"`
	HideDaysIsSet      int     `db:"hide_days_is_set"`
	AdvanceBook        int     `db:"advance_book"`
	CancelHours        int     `db:"cancel_hours"`
	ConfirmMinutes     int     `db:"confirm_minutes"`
	AvgConfirmTime     int     `db:"avg_confirm_time"`

	// Departure extras from trip_pool4_departure_extra
	NewTripFlag          int `db:"new_trip_flag"`
	SpecialDealFlag      int `db:"special_deal_flag"`
	RankScoreSales       float64 `db:"rank_score_sales"`
	RankScoreFormula     float64 `db:"rank_score_formula"`
	RankScoreFormulaRev  float64 `db:"rank_score_formula_revenue"`
	RankScoreSalesReal90 float64 `db:"rank_score_sales_real_90_days"`
	Bookings30d          int     `db:"bookings_30d"`
	Bookings30dSolo      int     `db:"bookings_30d_solo"`

	RoundTripDiscountPct float64 `db:"round_trip_discount_pct"`

	PriceBinStr []byte `db:"price_bin_str"`
	Godate      int64  `db:"godate"`
	GodateStamp int64  `db:"godate_stamp"`
	Dep         string `db:"dep"`
	Arr         string `db:"arr"`
}

func (row *rawTripRow) toDomainTrip() domain.RawTrip {
	return domain.RawTrip{
		TripKey: row.TripKey, Duration: row.Duration,
		DepartureTime: row.DepartureTime, Departure2Time: row.Departure2Time, Departure3Time: row.Departure3Time,
		ClassID: row.ClassID, OfficialID: row.OfficialID, OperatorID: row.OperatorID, VehclassID: row.VehclassID,
		DepStationID: row.DepStationID, ArrStationID: row.ArrStationID, SetID: row.SetID,
		DepTimezoneName: row.DepTimezoneName, ArrTimezoneName: row.ArrTimezoneName,
		DepCountryID: row.DepCountryID, ArrCountryID: row.ArrCountryID,
		DepProvinceID: row.DepProvinceID, ArrProvinceID: row.ArrProvinceID,
		DepHideDeparture: row.DepHideDeparture != 0,
		OpBookable: row.OpBookable != 0, SellerID: row.SellerID, MasterOperatorID: row.MasterOperatorID,
		PriceRestriction: row.PriceRestriction,
		IntegrationCode: row.IntegrationCode, IntegrationID: row.IntegrationID, ChunkKey: row.ChunkKey,
		Vehclasses: row.Vehclasses,
		RatingAvg: row.RatingAvg, RatingCount: row.RatingCount, SalesPerMonth: row.SalesPerMonth,
		BaggageFreeWeight: row.BaggageFreeWeight, BaggageFreeHand: row.BaggageFreeHand,
		BaggageFreeChecked: row.BaggageFreeChecked,
		Amenities: row.Amenities, TicketType: row.TicketType, SRMarker: row.SRMarker,
		IsMeta: row.IsMeta != 0, IsIgnoreGroupTime: row.IsIgnoreGroupTime != 0, IsFRefundable: row.IsFRefundable != 0,
		TripID: row.TripID, RouteID: row.RouteID,
		HideDays: row.HideDays, HideDaysIsSet: row.HideDaysIsSet != 0, AdvanceBook: row.AdvanceBook,
		CancelHours: row.CancelHours, ConfirmMinutes: row.ConfirmMinutes, AvgConfirmTime: row.AvgConfirmTime,
		NewTripFlag: row.NewTripFlag != 0, SpecialDealFlag: row.SpecialDealFlag != 0,
		RankScoreSales: row.RankScoreSales, RankScoreFormula: row.RankScoreFormula,
		RankScoreFormulaRev: row.RankScoreFormulaRev,
		RankScoreSalesReal90: row.RankScoreSalesReal90,
		Bookings30d: row.Bookings30d, Bookings30dSolo: row.Bookings30dSolo,
		RoundTripDiscountPct: row.RoundTripDiscountPct,
		PriceBinStr: row.PriceBinStr, Godate: row.Godate, GodateStamp: row.GodateStamp,
		Dep: row.Dep, Arr: row.Arr,
	}
}

func (r *TripPoolRepo) buildSearchQuery(p SearchParams) (string, []interface{}) {
	// Args must be ordered to match the left-to-right appearance of ? in the
	// final SQL text:  SELECT ? … FROM (VALUES ?) … JOIN … godate = ? WHERE …
	var selectArgs []interface{} // ? in SELECT clause
	var fromArgs []interface{}   // ? in FROM / JOIN clauses
	var whereArgs []interface{}  // ? in WHERE clause

	// --- SELECT clause params (appear first in SQL text) ---
	// price_5_6_pool params
	selectArgs = append(selectArgs, p.GodateString) // DATE(?)
	selectArgs = append(selectArgs, p.GodateString, p.SeatsAdult, p.SeatsChild, p.SeatsInfant,
		p.AgentID, p.Lang, p.FXCode, p.RecheckLevel, p.PriceMode)
	// godate, dep, arr
	selectArgs = append(selectArgs, p.GodateString) // godate CONVERT_TZ
	selectArgs = append(selectArgs, p.GodateString) // dep
	selectArgs = append(selectArgs, p.GodateString) // arr

	// --- FROM clause: route_place join for place-to-place ('p'), VALUES union otherwise ---
	// PHP uses trip_pool4_route_place → route_place_station → route_station → trip_pool4
	// for place-to-place searches (both IDs end in 'p'). This constrains results to
	// trips that belong to explicitly defined routes between those places.
	var fromClauseSQL string
	if strings.HasSuffix(p.FromPlaceID, "p") && strings.HasSuffix(p.ToPlaceID, "p") {
		fromPlaceNum, _ := strconv.Atoi(p.FromPlaceID[:len(p.FromPlaceID)-1])
		toPlaceNum, _ := strconv.Atoi(p.ToPlaceID[:len(p.ToPlaceID)-1])
		fromClauseSQL = `(
    SELECT DISTINCT tp.*
    FROM trip_pool4_route_place rp
    JOIN trip_pool4_route_place_station rps ON rps.route_place_id = rp.route_place_id
    JOIN trip_pool4_route_station rs ON rs.route_station_id = rps.route_station_id
    JOIN trip_pool4 tp ON tp.from_id = rs.from_id AND tp.to_id = rs.to_id
    WHERE rp.from_place_type = 'p' AND rp.from_place_id = ? AND rp.to_place_type = 'p' AND rp.to_place_id = ?
) tp`
		fromArgs = append(fromArgs, fromPlaceNum, toPlaceNum)
	} else {
		fromValues := buildValuesClause(p.FromStationIDs)
		toValues := buildValuesClause(p.ToStationIDs)
		fromClauseSQL = fmt.Sprintf("(%s) sf\nSTRAIGHT_JOIN trip_pool4 tp ON sf.station_id = tp.from_id\nSTRAIGHT_JOIN (%s) st ON st.station_id = tp.to_id",
			fromValues, toValues)
		for _, id := range p.FromStationIDs {
			fromArgs = append(fromArgs, id)
		}
		for _, id := range p.ToStationIDs {
			fromArgs = append(fromArgs, id)
		}
	}
	fromArgs = append(fromArgs, p.GodateString) // trip_price.godate = ?

	// WHERE clause filters
	var whereClauses []string
	whereClauses = append(whereClauses, "operator.bookable = 1", "seller.bookable = 1")

	if len(p.OperatorIDs) > 0 {
		whereClauses = append(whereClauses, "tp.operator_id IN ("+placeholders(len(p.OperatorIDs))+")")
		for _, id := range p.OperatorIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.ExcludeOperatorIDs) > 0 {
		whereClauses = append(whereClauses, "tp.operator_id NOT IN ("+placeholders(len(p.ExcludeOperatorIDs))+")")
		for _, id := range p.ExcludeOperatorIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.SellerIDs) > 0 {
		whereClauses = append(whereClauses, "operator.seller_id IN ("+placeholders(len(p.SellerIDs))+")")
		for _, id := range p.SellerIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.ExcludeSellerIDs) > 0 {
		whereClauses = append(whereClauses, "operator.seller_id NOT IN ("+placeholders(len(p.ExcludeSellerIDs))+")")
		for _, id := range p.ExcludeSellerIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.VehclassIDs) > 0 {
		whereClauses = append(whereClauses, "tp.vehclass_id IN ("+placeholders(len(p.VehclassIDs))+")")
		for _, id := range p.VehclassIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.ExcludeVehclassIDs) > 0 {
		whereClauses = append(whereClauses, "tp.vehclass_id NOT IN ("+placeholders(len(p.ExcludeVehclassIDs))+")")
		for _, id := range p.ExcludeVehclassIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.ClassIDs) > 0 {
		whereClauses = append(whereClauses, "tp.class_id IN ("+placeholders(len(p.ClassIDs))+")")
		for _, id := range p.ClassIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.ExcludeClassIDs) > 0 {
		whereClauses = append(whereClauses, "tp.class_id NOT IN ("+placeholders(len(p.ExcludeClassIDs))+")")
		for _, id := range p.ExcludeClassIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.CountryIDs) > 0 {
		whereClauses = append(whereClauses, "province_from.country_id IN ("+placeholders(len(p.CountryIDs))+")")
		for _, id := range p.CountryIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if len(p.ExcludeCountryIDs) > 0 {
		whereClauses = append(whereClauses, "province_from.country_id NOT IN ("+placeholders(len(p.ExcludeCountryIDs))+")")
		for _, id := range p.ExcludeCountryIDs {
			whereArgs = append(whereArgs, id)
		}
	}
	if p.IntegrationCode != "" {
		whereClauses = append(whereClauses, "COALESCE(integration.integration_code, 'manual') = ?")
		whereArgs = append(whereArgs, p.IntegrationCode)
	}
	if len(p.TripKeys) > 0 {
		whereClauses = append(whereClauses, "tp.trip_key IN ("+placeholders(len(p.TripKeys))+")")
		for _, key := range p.TripKeys {
			whereArgs = append(whereArgs, key)
		}
	}
	if p.OnlyDirect {
		whereClauses = append(whereClauses, "tp.set_id = 0")
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	query := fmt.Sprintf(`/* f4-search */
SELECT DISTINCT
    tp.trip_key,
    COALESCE(NULLIF(MAX(trip_price.duration), 0), tp.duration) AS duration,
    COALESCE(trip_price.departure_time, tp.departure_time) AS departure_time,
    COALESCE(trip_price.departure2_time, 0) AS departure2_time,
    COALESCE(trip_price.departure3_time, 0) AS departure3_time,
    tp.class_id, COALESCE(tp.official_id, '') AS official_id, tp.operator_id, COALESCE(tp.vehclass_id, '') AS vehclass_id,
    tp.from_id AS dep_station_id, tp.to_id AS arr_station_id,
    IF(tp.set_id = 0, NULL, tp.set_id) AS set_id,
    station_from.province_id AS dep_province_id,
    COALESCE(station_from.hide_departure, 0) AS dep_hide_departure,
    province_from.country_id AS dep_country_id,
    timezone_from.timezone_name AS dep_timezone_name,
    station_to.province_id AS arr_province_id,
    province_to.country_id AS arr_country_id,
    timezone_to.timezone_name AS arr_timezone_name,
    COALESCE(class.vehclasses, '') AS vehclasses,
    (operator.bookable AND seller.bookable) AS op_bookable,
    operator.seller_id, COALESCE(operator.master_id, 0) AS master_operator_id,
    COALESCE(integration.integration_code, 'manual') AS integration_code,
    COALESCE(integration.integration_id, 0) AS integration_id,
    COALESCE(integration.chunk_key, '') AS chunk_key,
    COALESCE(tpe.rating_avg, 0) AS rating_avg,
    COALESCE(tpe.rating_count, 0) AS rating_count,
    COALESCE(tpe.sales_per_month, 0) AS sales_per_month,
    COALESCE(tpe.baggage_free_weight, 0) AS baggage_free_weight,
    COALESCE(tpe.baggage_free_hand, 0) AS baggage_free_hand,
    COALESCE(tpe.baggage_free_checked, 0) AS baggage_free_checked,
    COALESCE(tpe.amenities, '') AS amenities,
    COALESCE(tpe.ticket_type, '') AS ticket_type,
    COALESCE(tpe.sr_marker, '') AS sr_marker,
    COALESCE(tpe.is_meta, 0) AS is_meta,
    COALESCE(tpe.is_ignore_group_time, 0) AS is_ignore_group_time,
    COALESCE(tpe.is_f_refundable, 0) AS is_f_refundable,
    COALESCE(tpe.trip_id, 0) AS trip_id,
    COALESCE(tpe.route_id, 0) AS route_id,
    COALESCE(tpe.hide_days, 0) AS hide_days,
    (tpe.hide_days IS NOT NULL) AS hide_days_is_set,
    COALESCE(tpe.advance_book, 0) AS advance_book,
    COALESCE(tpe.cancel_hours, 0) AS cancel_hours,
    COALESCE(tpe.confirm_minutes, 0) AS confirm_minutes,
    COALESCE(tpe.avg_confirm_time, 0) AS avg_confirm_time,
    COALESCE(seller.price_restriction, 0) AS price_restriction,
    COALESCE(tpd.new_trip_flag, 0) AS new_trip_flag,
    COALESCE(tpd.special_deal_flag, 0) AS special_deal_flag,
    COALESCE(tpd.rank_score_sales, 0) AS rank_score_sales,
    COALESCE(tpd.rank_score_formula, 0) AS rank_score_formula,
    COALESCE(tpd.rank_score_formula_revenue, 0) AS rank_score_formula_revenue,
    COALESCE(tpd.rank_score_sales_real_90_days, 0) AS rank_score_sales_real_90_days,
    COALESCE(tpd.bookings, 0) AS bookings_30d,
    COALESCE(tpd.bookings_solo, 0) AS bookings_30d_solo,
    MAX(COALESCE(trip_price.round_trip_discount_pct, 0)) AS round_trip_discount_pct,
    price_5_6_pool(
        tp.trip_key, DATE(?),
        UNIX_TIMESTAMP(CONVERT_TZ(
            CONCAT(?, ' ', SEC_TO_TIME(COALESCE(trip_price.departure_time, tp.departure_time) * 60)),
            timezone_from.timezone_name, 'UTC'
        )),
        ?, ?, ?, ?, ?, ?, ?, ?,
        trip_price.godate, trip_price.seats, 0, 0,
        trip_price.fxcode, trip_price.netprice, NULL, NULL,
        trip_price.topup_fxcode, trip_price.topup, NULL, NULL,
        trip_price.available_seats, trip_price.reason_id, trip_price.reason_param, trip_price.adv_book,
        UNIX_TIMESTAMP(trip_price.stamp),
        trip_price.duration * 60, trip_price.route_id, trip_price.trip_id
    ) AS price_bin_str,
    FLOOR(UNIX_TIMESTAMP(CONVERT_TZ(
        CONCAT(?, ' ', SEC_TO_TIME(COALESCE(trip_price.departure_time, tp.departure_time) * 60)),
        timezone_from.timezone_name, 'UTC'
    ))) AS godate,
    COALESCE(UNIX_TIMESTAMP(trip_price.stamp), 0) AS godate_stamp,
    CONCAT(?, ' ', SEC_TO_TIME(COALESCE(trip_price.departure_time, tp.departure_time) * 60)) AS dep,
    DATE_ADD(
        CONCAT(?, ' ', SEC_TO_TIME(COALESCE(trip_price.departure_time, tp.departure_time) * 60)),
        INTERVAL COALESCE(NULLIF(MAX(trip_price.duration), 0), tp.duration) MINUTE
    ) AS arr
FROM %s
JOIN station station_from ON station_from.station_id = tp.from_id
JOIN province province_from ON province_from.province_id = station_from.province_id
JOIN timezone timezone_from ON timezone_from.timezone_id = province_from.timezone_id
JOIN station station_to ON station_to.station_id = tp.to_id
JOIN province province_to ON province_to.province_id = station_to.province_id
JOIN timezone timezone_to ON timezone_to.timezone_id = province_to.timezone_id
JOIN class ON class.class_id = tp.class_id
JOIN operator ON operator.operator_id = tp.operator_id
JOIN seller ON seller.seller_id = operator.seller_id
LEFT JOIN integration ON integration.seller_id = operator.seller_id
LEFT JOIN trip_pool4_extra tpe ON tpe.trip_key = tp.trip_key
LEFT JOIN trip_pool4_price trip_price ON trip_price.trip_key = tp.trip_key AND trip_price.godate = ?
LEFT JOIN trip_pool4_departure_extra tpd ON tpd.trip_key = trip_price.trip_key
    AND tpd.departure_time = trip_price.departure_time
    AND tpd.departure2_time = trip_price.departure2_time
    AND tpd.departure3_time = trip_price.departure3_time
WHERE %s
GROUP BY tp.trip_key, trip_price.departure_time, trip_price.departure2_time, trip_price.departure3_time`,
		fromClauseSQL, whereSQL)

	if p.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", p.Limit)
	}

	// Combine args in SQL textual order: SELECT ? … FROM (VALUES ?) … JOIN godate=? … WHERE ?
	args := make([]interface{}, 0, len(selectArgs)+len(fromArgs)+len(whereArgs))
	args = append(args, selectArgs...)
	args = append(args, fromArgs...)
	args = append(args, whereArgs...)

	return query, args
}

func buildValuesClause(ids []int) string {
	parts := make([]string, 0, len(ids)+1)
	parts = append(parts, "SELECT NULL AS station_id")
	for range ids {
		parts = append(parts, "SELECT ?")
	}
	return strings.Join(parts, " UNION ")
}

func placeholders(n int) string {
	if n == 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}
