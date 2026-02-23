package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// RoundTripPriceRow represents a cached round trip price from DB.
type RoundTripPriceRow struct {
	OutboundTripKey    string `db:"outbound_trip_key"`
	OutboundGodatetime string `db:"outbound_godatetime"`
	InboundTripKey     string `db:"inbound_trip_key"`
	InboundGodate      string `db:"inbound_godate"`
	InboundDepartureTime int  `db:"inbound_departure_time"`
	PriceBinStr        []byte `db:"price_bin_str"`
	InboundPriceBinStr []byte `db:"inbound_price_bin_str"`
	Seats              int    `db:"seats"`
	FXCode             string `db:"fxcode"`
	AvailableSeats     int    `db:"available_seats"`
}

// RoundTripPriceRepo queries the trip_pool4_round_trip_price table.
type RoundTripPriceRepo struct {
	getDB func(region string) *sqlx.DB
}

func NewRoundTripPriceRepo(getDB func(region string) *sqlx.DB) *RoundTripPriceRepo {
	return &RoundTripPriceRepo{getDB: getDB}
}

// FindByOutbound returns cached round trip prices for an outbound trip.
func (r *RoundTripPriceRepo) FindByOutbound(ctx context.Context, region, outboundTripKey string, outboundGodate string) ([]RoundTripPriceRow, error) {
	db := r.getDB(region)
	var rows []RoundTripPriceRow
	err := db.SelectContext(ctx, &rows, `
		SELECT outbound_trip_key, outbound_godatetime, inbound_trip_key,
		       inbound_godate, inbound_departure_time, price_bin_str,
		       inbound_price_bin_str, seats, fxcode, available_seats
		FROM trip_pool4_round_trip_price
		WHERE outbound_trip_key = ? AND outbound_godatetime LIKE ?
		  AND TIMESTAMPDIFF(SECOND, stamp, NOW()) < 1200`,
		outboundTripKey, outboundGodate+"%")
	return rows, err
}
