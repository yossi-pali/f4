package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/12go/f4/internal/domain"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyName converts a station name to a URL-safe slug (matching PHP Slugger).
func slugifyName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// buildStationNameFull computes the full station name (matching PHP StationManager::getStationNameFull).
// Format: "CODE StationName, ProvinceName" (code only for avia, province only if not already in name).
// Province check is case-sensitive to match PHP strpos behavior.
func buildStationNameFull(stationName, provinceName, vehclassID, stationCode string) string {
	name := stationName
	if provinceName != "" && !strings.Contains(stationName, provinceName) {
		name = name + ", " + provinceName
	}
	if vehclassID == "avia" && stationCode != "" {
		name = stationCode + " " + name
	}
	return name
}

// StationRepo handles station, province, and place resolution queries.
type StationRepo struct {
	db *sqlx.DB
}

func NewStationRepo(db *sqlx.DB) *StationRepo {
	return &StationRepo{db: db}
}

// stationRow extends Station with province_name for computing derived fields.
type stationRow struct {
	domain.Station
	ProvinceName string `db:"province_name"`
}

func (row stationRow) toStation() domain.Station {
	s := row.Station
	s.StationSlug = slugifyName(s.StationName)
	code := ""
	if s.StationCode != nil {
		code = *s.StationCode
	}
	s.StationNameFull = buildStationNameFull(s.StationName, row.ProvinceName, s.VehclassID, code)
	return s
}

// FindStationByID returns a single station by ID.
func (r *StationRepo) FindStationByID(ctx context.Context, id int) (domain.Station, error) {
	var row stationRow
	err := r.db.GetContext(ctx, &row, `
		SELECT s.station_id, COALESCE(s.station_name, '') AS station_name,
		       s.station_code,
		       COALESCE(s.lat, 0) AS lat, COALESCE(s.lng, 0) AS lng,
		       s.province_id, COALESCE(p.country_id, '') AS country_id,
		       COALESCE(s.vehclass_id, '') AS vehclass_id, p.timezone_id,
		       tz.timezone_name, COALESCE(s.weight_from, 0) AS weight_from,
		       COALESCE(s.coordinates_accurate, 0) AS coordinates_accurate,
		       COALESCE(p.province_name, '') AS province_name
		FROM station s
		JOIN province p ON p.province_id = s.province_id
		JOIN timezone tz ON tz.timezone_id = p.timezone_id
		WHERE s.station_id = ?`, id)
	if err != nil {
		return domain.Station{}, err
	}
	return row.toStation(), nil
}

// FindStationsByIDs returns stations by IDs as a map.
func (r *StationRepo) FindStationsByIDs(ctx context.Context, ids []int) (map[int]domain.Station, error) {
	if len(ids) == 0 {
		return map[int]domain.Station{}, nil
	}
	query, args, err := sqlx.In(`
		SELECT s.station_id, COALESCE(s.station_name, '') AS station_name,
		       s.station_code,
		       COALESCE(s.lat, 0) AS lat, COALESCE(s.lng, 0) AS lng,
		       s.province_id, COALESCE(p.country_id, '') AS country_id,
		       COALESCE(s.vehclass_id, '') AS vehclass_id, p.timezone_id,
		       tz.timezone_name, COALESCE(s.weight_from, 0) AS weight_from,
		       COALESCE(s.coordinates_accurate, 0) AS coordinates_accurate,
		       COALESCE(p.province_name, '') AS province_name
		FROM station s
		JOIN province p ON p.province_id = s.province_id
		JOIN timezone tz ON tz.timezone_id = p.timezone_id
		WHERE s.station_id IN (?)`, ids)
	if err != nil {
		return nil, err
	}

	var rows []stationRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}

	result := make(map[int]domain.Station, len(rows))
	for _, row := range rows {
		s := row.toStation()
		result[s.StationID] = s
	}
	return result, nil
}

// FindProvinceByID returns a single province by ID.
func (r *StationRepo) FindProvinceByID(ctx context.Context, id int) (domain.Province, error) {
	var p domain.Province
	err := r.db.GetContext(ctx, &p, `
		SELECT province_id, province_name, lat, lng, parent_id, country_id
		FROM province
		WHERE province_id = ?`, id)
	return p, err
}

// FindSearchStationIDs returns station IDs for a given search_station_id (station place).
func (r *StationRepo) FindSearchStationIDs(ctx context.Context, searchStationID int) ([]int, error) {
	var ids []int
	err := r.db.SelectContext(ctx, &ids,
		`SELECT station_id FROM search_station WHERE search_station_id = ?`, searchStationID)
	return ids, err
}

// FindSearchProvinceStationIDs returns station IDs for a given search_province_id (province place).
func (r *StationRepo) FindSearchProvinceStationIDs(ctx context.Context, searchProvinceID int) ([]int, error) {
	var ids []int
	err := r.db.SelectContext(ctx, &ids,
		`SELECT station_id FROM search_province WHERE search_province_id = ?`, searchProvinceID)
	return ids, err
}

// ResolvePlaceToStationIDs resolves a place ID (e.g. "123p" or "456s") to station IDs.
func (r *StationRepo) ResolvePlaceToStationIDs(ctx context.Context, placeID string) ([]int, error) {
	if placeID == domain.UnknownPlace {
		return nil, nil
	}

	suffix := placeID[len(placeID)-1:]
	numStr := placeID[:len(placeID)-1]
	var id int
	if _, err := fmt.Sscanf(numStr, "%d", &id); err != nil {
		return nil, fmt.Errorf("invalid place ID format: %s", placeID)
	}

	if suffix == "s" {
		return r.FindSearchStationIDs(ctx, id)
	}
	return r.FindSearchProvinceStationIDs(ctx, id)
}

// GetPlaceData returns basic place data (name, lat, lng) for a place ID.
func (r *StationRepo) GetPlaceData(ctx context.Context, placeID string) (domain.Place, error) {
	if placeID == domain.UnknownPlace {
		return domain.Place{}, nil
	}

	suffix := placeID[len(placeID)-1:]
	numStr := placeID[:len(placeID)-1]
	var id int
	if _, err := fmt.Sscanf(numStr, "%d", &id); err != nil {
		return domain.Place{}, fmt.Errorf("invalid place ID format: %s", placeID)
	}

	place := domain.Place{ID: id}

	if suffix == "s" {
		place.Type = domain.PlaceTypeStation
		s, err := r.FindStationByID(ctx, id)
		if err != nil {
			return place, err
		}
		place.Name = s.StationName
		place.Lat = s.Lat
		place.Lng = s.Lng
		place.ProvinceID = s.ProvinceID
	} else {
		place.Type = domain.PlaceTypeProvince
		p, err := r.FindProvinceByID(ctx, id)
		if err != nil {
			return place, err
		}
		place.Name = p.ProvinceName
		place.Lat = p.Lat
		place.Lng = p.Lng
		if p.ParentID != nil {
			place.ParentID = *p.ParentID
		}
	}

	return place, nil
}

// GetParentProvinceName returns the province name for a place ID, navigating to parent if available.
// Matches PHP PlaceManager::getParentProvinceName — if province has parent_id, return parent name;
// otherwise return province's own name.
func (r *StationRepo) GetParentProvinceName(ctx context.Context, placeID string) string {
	if len(placeID) < 2 {
		return ""
	}
	suffix := placeID[len(placeID)-1:]
	numStr := placeID[:len(placeID)-1]
	var id int
	if _, err := fmt.Sscanf(numStr, "%d", &id); err != nil {
		return ""
	}

	// If it's a station, resolve to its province first
	if suffix == "s" {
		var provinceID int
		if err := r.db.GetContext(ctx, &provinceID,
			`SELECT province_id FROM station WHERE station_id = ?`, id); err != nil {
			return ""
		}
		id = provinceID
	}

	// Get province, navigate to parent if exists
	var p struct {
		ProvinceName string `db:"province_name"`
		ParentID     *int   `db:"parent_id"`
	}
	if err := r.db.GetContext(ctx, &p,
		`SELECT province_name, parent_id FROM province WHERE province_id = ?`, id); err != nil {
		return ""
	}

	if p.ParentID != nil {
		var parentName string
		if err := r.db.GetContext(ctx, &parentName,
			`SELECT province_name FROM province WHERE province_id = ?`, *p.ParentID); err == nil {
			return parentName
		}
	}

	return p.ProvinceName
}

// GetTimezoneByStationID returns the timezone name for a station.
func (r *StationRepo) GetTimezoneByStationID(ctx context.Context, stationID int) (string, error) {
	var tz string
	err := r.db.GetContext(ctx, &tz, `
		SELECT tz.timezone_name
		FROM station s
		JOIN province p ON p.province_id = s.province_id
		JOIN timezone tz ON tz.timezone_id = p.timezone_id
		WHERE s.station_id = ?`, stationID)
	return tz, err
}

// helper to build IN clause placeholders
func inPlaceholders(n int) string {
	if n == 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}
