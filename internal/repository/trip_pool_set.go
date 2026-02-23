package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// TripPoolSet represents a multi-leg trip set from trip_pool4_set.
type TripPoolSet struct {
	SetID             int     `db:"set_id"`
	TripKey           string  `db:"trip_key"`
	Trip1Key          string  `db:"trip1_key"`
	Trip2Key          string  `db:"trip2_key"`
	Trip3Key          *string `db:"trip3_key"`
	PackID            int     `db:"pack_id"`
	Transit1Guarantee int     `db:"transit1_guarantee"`
	Transit2Guarantee int     `db:"transit2_guarantee"`
	Trip2Day          int     `db:"trip2_day"`
	Trip3Day          int     `db:"trip3_day"`
}

// TripPoolSetRepo handles queries on the trip_pool4_set table.
type TripPoolSetRepo struct {
	connMgr *ConnectionManagerRef
}

type ConnectionManagerRef struct {
	getDB func(region string) *sqlx.DB
}

func NewTripPoolSetRepo(getDB func(region string) *sqlx.DB) *TripPoolSetRepo {
	return &TripPoolSetRepo{connMgr: &ConnectionManagerRef{getDB: getDB}}
}

// FindBySetIDs returns trip sets by their set IDs.
func (r *TripPoolSetRepo) FindBySetIDs(ctx context.Context, region string, setIDs []int) ([]TripPoolSet, error) {
	if len(setIDs) == 0 {
		return nil, nil
	}
	db := r.connMgr.getDB(region)
	query, args, err := sqlx.In(
		`SELECT set_id, trip_key, trip1_key, trip2_key, trip3_key,
		        COALESCE(pack_id, 0) AS pack_id,
		        COALESCE(transit1_guarantee, 0) AS transit1_guarantee,
		        COALESCE(transit2_guarantee, 0) AS transit2_guarantee,
		        COALESCE(trip2_day, 0) AS trip2_day,
		        COALESCE(trip3_day, 0) AS trip3_day
		 FROM trip_pool4_set WHERE set_id IN (?)`, setIDs)
	if err != nil {
		return nil, err
	}
	var sets []TripPoolSet
	err = db.SelectContext(ctx, &sets, db.Rebind(query), args...)
	return sets, err
}
