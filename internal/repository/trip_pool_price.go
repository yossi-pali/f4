package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/12go/f4/internal/db"
	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/price"
)

// TripPoolPriceRepo handles price prediction queries for trips with PriceNone.
type TripPoolPriceRepo struct {
	connMgr *db.ConnectionManager
	regionR db.RegionResolver
}

func NewTripPoolPriceRepo(connMgr *db.ConnectionManager, regionR db.RegionResolver) *TripPoolPriceRepo {
	return &TripPoolPriceRepo{connMgr: connMgr, regionR: regionR}
}

// PredictedPriceRow is a simplified row for price prediction.
type PredictedPriceRow struct {
	TripKey     string `db:"trip_key"`
	PriceBinStr []byte `db:"price_bin_str"`
}

// FindPredictedPrices queries adjacent dates (±1-2 days) to find prices for trips
// that have PriceNone on the requested date.
func (r *TripPoolPriceRepo) FindPredictedPrices(ctx context.Context, region string, tripKeys []string, godate string) (map[string]domain.TripPrice, error) {
	if len(tripKeys) == 0 {
		return map[string]domain.TripPrice{}, nil
	}

	dbConn := r.connMgr.TripPool(region)
	if dbConn == nil {
		return nil, fmt.Errorf("no DB connection for region %s", region)
	}

	// Build placeholders for trip keys
	placeholders := make([]string, len(tripKeys))
	args := make([]any, 0, len(tripKeys)+1)
	for i, key := range tripKeys {
		placeholders[i] = "?"
		args = append(args, key)
	}
	args = append(args, godate)

	// Query adjacent dates for price prediction
	query := fmt.Sprintf(`
		SELECT tp.trip_key,
		       price_5_6_pool(tpp.price_bin_str, tpp.seats, tpp.fxcode, 0, 1, 0, 0) AS price_bin_str
		FROM trip_pool4 tp
		JOIN trip_pool4_price tpp ON tpp.trip_key = tp.trip_key
		WHERE tp.trip_key IN (%s)
		  AND tpp.godate BETWEEN DATE_SUB(?, INTERVAL 2 DAY) AND DATE_ADD(?, INTERVAL 2 DAY)
		  AND tpp.godate != ?
		  AND TIMESTAMPDIFF(SECOND, tpp.stamp, NOW()) < 86400
		ORDER BY ABS(DATEDIFF(tpp.godate, ?))
	`, strings.Join(placeholders, ","))

	// Add the remaining date args
	args = append(args, godate, godate, godate)

	var rows []PredictedPriceRow
	if err := dbConn.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	// Take the first valid price found per trip key (nearest date)
	result := make(map[string]domain.TripPrice, len(rows))
	for _, row := range rows {
		if _, ok := result[row.TripKey]; ok {
			continue // already found a price for this trip key
		}
		if len(row.PriceBinStr) > 0 {
			tp, err := price.Decode(row.PriceBinStr)
			if err == nil && tp.IsValid {
				tp.PriceLevel = domain.PricePredict
				result[row.TripKey] = tp
			}
		}
	}

	return result, nil
}
