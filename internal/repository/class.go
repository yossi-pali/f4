package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/12go/f4/internal/domain"
)

// ClassRepo handles vehicle class queries.
type ClassRepo struct {
	db *sqlx.DB
}

func NewClassRepo(db *sqlx.DB) *ClassRepo {
	return &ClassRepo{db: db}
}

// FindByIDs returns classes by IDs as a map.
func (r *ClassRepo) FindByIDs(ctx context.Context, ids []int) (map[int]domain.VehicleClass, error) {
	if len(ids) == 0 {
		return map[int]domain.VehicleClass{}, nil
	}
	query, args, err := sqlx.In(`
		SELECT class_id, COALESCE(class_name, '') AS class_name,
		       COALESCE(vehclasses, '') AS vehclasses,
		       COALESCE(is_multi_pax, 0) AS is_multi_pax,
		       COALESCE(seats, 0) AS seats
		FROM class
		WHERE class_id IN (?)`, ids)
	if err != nil {
		return nil, err
	}

	var classes []domain.VehicleClass
	if err := r.db.SelectContext(ctx, &classes, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}

	result := make(map[int]domain.VehicleClass, len(classes))
	for _, c := range classes {
		result[c.ClassID] = c
	}
	return result, nil
}
