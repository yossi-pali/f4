package db

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
)

const (
	DefaultRegion  = "default"
	regionTypeTp   = "tp"
)

// RegionResolver maps station or place IDs to their geographic database region.
type RegionResolver interface {
	ResolveByStationID(stationID int) string
	ResolveByPlaceID(placeID string) string
	ResolveByCountryID(countryID string) string
	ResolveByTripKey(tripKey string) string
}

// DBRegionResolver resolves regions by looking up country_id from station/province,
// then mapping country_id → region_id via the country_region table (type='tp').
// Ported from PHP ProductPoolRegion.
type DBRegionResolver struct {
	db              *sqlx.DB
	mu              sync.RWMutex
	regionByCountry map[string]string // uppercase country_id → region_id
	loaded          bool
}

func NewDBRegionResolver(db *sqlx.DB) *DBRegionResolver {
	return &DBRegionResolver{
		db:              db,
		regionByCountry: make(map[string]string),
	}
}

// Init loads the country→region mapping from the country_region table.
// Should be called once at startup.
func (r *DBRegionResolver) Init(ctx context.Context) error {
	type row struct {
		CountryID string `db:"country_id"`
		RegionID  string `db:"region_id"`
	}
	var rows []row
	err := r.db.SelectContext(ctx, &rows,
		`SELECT country_id, region_id FROM country_region WHERE type = ?`, regionTypeTp)
	if err != nil {
		return fmt.Errorf("load country_region: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range rows {
		r.regionByCountry[strings.ToUpper(row.CountryID)] = row.RegionID
	}
	r.loaded = true
	return nil
}

func (r *DBRegionResolver) ResolveByCountryID(countryID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if region, ok := r.regionByCountry[strings.ToUpper(countryID)]; ok {
		return region
	}
	return DefaultRegion
}

func (r *DBRegionResolver) ResolveByStationID(stationID int) string {
	var countryID string
	err := r.db.Get(&countryID, `
		SELECT p.country_id
		FROM station s
		JOIN province p ON p.province_id = s.province_id
		WHERE s.station_id = ?`, stationID)
	if err != nil || countryID == "" {
		return DefaultRegion
	}
	return r.ResolveByCountryID(countryID)
}

func (r *DBRegionResolver) ResolveByPlaceID(placeID string) string {
	if len(placeID) < 2 {
		return DefaultRegion
	}
	suffix := placeID[len(placeID)-1:]
	numStr := placeID[:len(placeID)-1]
	var id int
	if _, err := fmt.Sscanf(numStr, "%d", &id); err != nil {
		return DefaultRegion
	}

	var countryID string
	if suffix == "s" {
		_ = r.db.Get(&countryID, `
			SELECT p.country_id
			FROM station s
			JOIN province p ON p.province_id = s.province_id
			WHERE s.station_id = ?`, id)
	} else {
		_ = r.db.Get(&countryID, `
			SELECT country_id FROM province WHERE province_id = ?`, id)
	}
	if countryID == "" {
		return DefaultRegion
	}
	return r.ResolveByCountryID(countryID)
}

func (r *DBRegionResolver) ResolveByTripKey(tripKey string) string {
	if len(tripKey) < 2 {
		return DefaultRegion
	}
	return r.ResolveByCountryID(tripKey[:2])
}

// StaticRegionResolver is a fallback that returns a fixed default region.
// Used when no database is available (e.g., testing).
type StaticRegionResolver struct {
	DefaultRegion string
}

func NewStaticRegionResolver(defaultRegion string) *StaticRegionResolver {
	return &StaticRegionResolver{DefaultRegion: defaultRegion}
}

func (r *StaticRegionResolver) ResolveByStationID(_ int) string    { return r.DefaultRegion }
func (r *StaticRegionResolver) ResolveByPlaceID(_ string) string   { return r.DefaultRegion }
func (r *StaticRegionResolver) ResolveByCountryID(_ string) string { return r.DefaultRegion }
func (r *StaticRegionResolver) ResolveByTripKey(_ string) string   { return r.DefaultRegion }
