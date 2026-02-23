package repository

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"github.com/12go/f4/internal/domain"
)

// AutopackRepo handles autopack configuration queries from landing_alternatives.
type AutopackRepo struct {
	db *sqlx.DB
}

func NewAutopackRepo(db *sqlx.DB) *AutopackRepo {
	return &AutopackRepo{db: db}
}

type autopackRow struct {
	ID            int    `db:"id"`
	FromPlaceID   int    `db:"from_place_id"`
	FromPlaceType string `db:"from_place_type"`
	ToPlaceID     int    `db:"to_place_id"`
	ToPlaceType   string `db:"to_place_type"`
	RoutesJSON    string `db:"routes_json"`
	IsActive      bool   `db:"is_active"`
}

// FindByPlaces returns autopack configurations for a from/to place pair.
func (r *AutopackRepo) FindByPlaces(ctx context.Context, fromPlaceID, toPlaceID string) ([]domain.AutopackConfig, error) {
	var rows []autopackRow
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, from_place_id, from_place_type, to_place_id, to_place_type, routes_json, is_active
		FROM landing_alternatives
		WHERE is_active = 1
		  AND CONCAT(from_place_id, from_place_type) = ?
		  AND CONCAT(to_place_id, to_place_type) = ?`, fromPlaceID, toPlaceID)
	if err != nil {
		return nil, err
	}

	configs := make([]domain.AutopackConfig, 0, len(rows))
	for _, row := range rows {
		cfg := domain.AutopackConfig{
			AutopackID:  row.ID,
			FromPlaceID: fromPlaceID,
			ToPlaceID:   toPlaceID,
			IsActive:    row.IsActive,
		}
		if row.RoutesJSON != "" {
			_ = json.Unmarshal([]byte(row.RoutesJSON), &cfg.Routes)
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}
